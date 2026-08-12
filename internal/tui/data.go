package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/opc"
)

// Every message below is the result of one OPC UA call, delivered back to the
// update loop. The loop itself never blocks on the network.

// levelMsg carries the children of a node.
type levelMsg struct {
	parent string
	nodes  []cligen.Node
	err    error
}

// valuesMsg carries refreshed values for the variables of the current level.
type valuesMsg struct {
	parent string
	values map[string]cligen.ReadResult
}

// attributesMsg carries the attributes of the selected node.
type attributesMsg struct {
	nodeID string
	set    cligen.AttributeSet
	err    error
}

// typeMsg carries the type description of the selected node.
type typeMsg struct {
	nodeID string
	info   cligen.TypeInfo
	err    error
}

// referencesMsg carries the references of the selected node.
type referencesMsg struct {
	nodeID     string
	references []cligen.Reference
	err        error
}

// watchMsg carries one live value from the watch subscription.
type watchMsg struct {
	nodeID string
	value  *ua.DataValue
}

// watchErrMsg reports that the watch subscription has stopped.
type watchErrMsg struct{ err error }

// tickMsg drives the auto-refresh.
type tickMsg time.Time

// statusMsg sets the footer message.
type statusMsg string

// browseLevel loads the children of a node.
func (m *model) browseLevel(target *ua.NodeID) tea.Cmd {
	client := m.client
	ctx := m.ctx

	return func() tea.Msg {
		nodes, err := opc.Browse(ctx, client, opc.BrowseOptions{
			Root:            target,
			Depth:           1,
			Direction:       ua.BrowseDirectionForward,
			IncludeSubtypes: true,
		})
		return levelMsg{parent: target.String(), nodes: nodes, err: err}
	}
}

// readValues reads the value of every variable in a level, in one call. The
// explorer shows values inline, and re-reading the whole level at once is what
// keeps that affordable.
func (m *model) readValues(parent string, nodes []cligen.Node) tea.Cmd {
	var targets []*ua.NodeID
	for _, node := range nodes {
		if node.NodeClass != "Variable" {
			continue
		}
		parsed, err := opc.ParseNodeID(node.NodeID)
		if err != nil {
			continue
		}
		targets = append(targets, parsed)
	}
	if len(targets) == 0 {
		return nil
	}

	client := m.client
	ctx := m.ctx

	return func() tea.Msg {
		results, err := opc.Read(ctx, client, opc.ReadOptions{
			Nodes:      targets,
			Attributes: []ua.AttributeID{ua.AttributeIDValue},
			Timestamps: ua.TimestampsToReturnBoth,
		})
		if err != nil {
			return statusMsg("read values: " + err.Error())
		}

		values := make(map[string]cligen.ReadResult, len(results))
		for _, result := range results {
			values[result.NodeID] = result
		}
		return valuesMsg{parent: parent, values: values}
	}
}

// loadAttributes reads every attribute of one node.
func (m *model) loadAttributes(nodeID string) tea.Cmd {
	client := m.client
	ctx := m.ctx

	return func() tea.Msg {
		target, err := opc.ParseNodeID(nodeID)
		if err != nil {
			return attributesMsg{nodeID: nodeID, err: err}
		}
		set, err := opc.Attributes(ctx, client, target, false)
		return attributesMsg{nodeID: nodeID, set: set, err: err}
	}
}

// loadType describes the data type of one node.
func (m *model) loadType(nodeID string) tea.Cmd {
	client := m.client
	ctx := m.ctx

	return func() tea.Msg {
		target, err := opc.ParseNodeID(nodeID)
		if err != nil {
			return typeMsg{nodeID: nodeID, err: err}
		}

		// A variable's interesting type is the data type it declares, not the
		// variable node itself, so the data type node is the one described.
		if declared, err := opc.TypeOf(ctx, client, target); err == nil {
			if dataType, err := opc.ParseNodeID(declared.DataTypeID); err == nil {
				info, err := opc.Describe(ctx, client, dataType, true)
				return typeMsg{nodeID: nodeID, info: info, err: err}
			}
		}

		info, err := opc.Describe(ctx, client, target, true)
		return typeMsg{nodeID: nodeID, info: info, err: err}
	}
}

// loadReferences lists the references of one node.
func (m *model) loadReferences(nodeID string) tea.Cmd {
	client := m.client
	ctx := m.ctx

	return func() tea.Msg {
		target, err := opc.ParseNodeID(nodeID)
		if err != nil {
			return referencesMsg{nodeID: nodeID, err: err}
		}
		references, err := opc.References(ctx, client, opc.ReferenceOptions{
			Node:            target,
			Direction:       ua.BrowseDirectionBoth,
			IncludeSubtypes: true,
		})
		return referencesMsg{nodeID: nodeID, references: references, err: err}
	}
}

// startWatch creates the subscription the watch panel feeds from. It is created
// on demand, so an explorer that is only browsing costs the server nothing.
func (m *model) startWatch() error {
	if m.subscription != nil {
		return nil
	}

	m.notifications = make(chan *opcua.PublishNotificationData, 32)
	sub, err := m.client.Subscribe(m.ctx, &opcua.SubscriptionParameters{
		Interval: m.refresh,
	}, m.notifications)
	if err != nil {
		return err
	}
	m.subscription = sub
	return nil
}

// addWatch monitors one more node on the watch subscription.
func (m *model) addWatch(target *ua.NodeID) error {
	if err := m.startWatch(); err != nil {
		return err
	}

	handle := uint32(len(m.watchOrder) + 1)
	m.watchHandles[handle] = target.String()

	_, err := m.subscription.Monitor(m.ctx, ua.TimestampsToReturnBoth,
		opcua.NewMonitoredItemCreateRequestWithDefaults(target, ua.AttributeIDValue, handle))
	return err
}

// listenWatch waits for the next notification. It is re-issued after every
// message, which is how a channel is consumed inside the update loop without
// blocking it.
func (m *model) listenWatch() tea.Cmd {
	notifications := m.notifications
	if notifications == nil {
		return nil
	}
	handles := m.watchHandles

	return func() tea.Msg {
		notification, ok := <-notifications
		if !ok {
			return watchErrMsg{err: context.Canceled}
		}
		if notification.Error != nil {
			return watchErrMsg{err: notification.Error}
		}

		change, ok := notification.Value.(*ua.DataChangeNotification)
		if !ok || len(change.MonitoredItems) == 0 {
			return statusMsg("")
		}

		item := change.MonitoredItems[0]
		return watchMsg{nodeID: handles[item.ClientHandle], value: item.Value}
	}
}

// tick schedules the next auto-refresh.
func (m *model) tick() tea.Cmd {
	return tea.Tick(m.refresh, func(t time.Time) tea.Msg { return tickMsg(t) })
}
