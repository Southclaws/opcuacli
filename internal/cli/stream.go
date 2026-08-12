package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/conn"
	"github.com/Southclaws/opcuacli/internal/opc"
	"github.com/Southclaws/opcuacli/internal/render"
)

// monitor subscribes to nodes and streams their changes until interrupted.
func monitor(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.MonitorParams) error {
	targets, err := nodes(p.Node, p.Stdin, commandIO.In)
	if err != nil {
		return err
	}

	settings, _, err := conn.FromCommand(cmd, commandIO.In)
	if err != nil {
		return err
	}
	streams := newOutput(cmd, commandIO)

	interval := p.Interval
	if !cmd.Flags().Changed("interval") && settings.SubscriptionInterval > 0 {
		interval = settings.SubscriptionInterval
	}
	queueSize := uint32(p.QueueSize)
	if !cmd.Flags().Changed("queue-size") && settings.QueueSize > 0 {
		queueSize = settings.QueueSize
	}

	filter, err := dataChangeFilter(string(p.Trigger), string(p.Deadband), p.DeadbandValue)
	if err != nil {
		return err
	}

	sampling := p.Sampling
	if sampling <= 0 {
		sampling = interval
	}

	requests := make([]*ua.MonitoredItemCreateRequest, 0, len(targets))
	for index, target := range targets {
		requests = append(requests, &ua.MonitoredItemCreateRequest{
			ItemToMonitor: &ua.ReadValueID{
				NodeID:       target,
				AttributeID:  ua.AttributeIDValue,
				DataEncoding: &ua.QualifiedName{},
			},
			MonitoringMode: monitoringMode(string(p.MonitoringMode)),
			RequestedParameters: &ua.MonitoringParameters{
				// The client handle is how a notification is matched back to the
				// node that produced it; the index into the request list is the
				// simplest handle that stays valid across a re-subscription.
				ClientHandle:     uint32(index + 1),
				SamplingInterval: float64(sampling / time.Millisecond),
				QueueSize:        queueSize,
				DiscardOldest:    p.DiscardOldest,
				Filter:           filter,
			},
		})
	}

	view := newStreamView(streams, string(p.Format), targets, p.Stats)
	defer view.close()

	limit := &streamLimit{count: p.Count, deadline: p.Duration}
	stream := &subscription{
		settings: settings,
		streams:  streams,
		view:     view,
		limit:    limit,
		parameters: &opcua.SubscriptionParameters{
			Interval:          interval,
			LifetimeCount:     uint32(p.LifetimeCount),
			MaxKeepAliveCount: uint32(p.KeepaliveCount),
			Priority:          uint8(min(max(p.Priority, 0), 255)),
		},
		timestamps: opc.TimestampsToReturn(string(p.Timestamps)),
		requests:   requests,
		handles:    targets,
	}

	return stream.run(ctx)
}

// events subscribes to the event notifier of a node and streams events.
func events(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.EventsParams) error {
	target, err := node(p.Node)
	if err != nil {
		return err
	}
	eventType, err := node(p.EventType)
	if err != nil {
		return err
	}
	severity, err := severityFilter(p.SeverityMin)
	if err != nil {
		return err
	}

	fields := p.Field
	if len(fields) == 0 {
		fields = opc.DefaultEventFields()
	}

	settings, _, err := conn.FromCommand(cmd, commandIO.In)
	if err != nil {
		return err
	}
	streams := newOutput(cmd, commandIO)

	filter := opc.EventFilterObject(opc.EventFilter(fields, eventType, severity))
	requests := []*ua.MonitoredItemCreateRequest{{
		ItemToMonitor: &ua.ReadValueID{
			NodeID:       target,
			AttributeID:  ua.AttributeIDEventNotifier,
			DataEncoding: &ua.QualifiedName{},
		},
		MonitoringMode: ua.MonitoringModeReporting,
		RequestedParameters: &ua.MonitoringParameters{
			ClientHandle:     1,
			SamplingInterval: float64(p.Interval / time.Millisecond),
			QueueSize:        uint32(p.QueueSize),
			DiscardOldest:    true,
			Filter:           filter,
		},
	}}

	view := newEventView(streams, string(p.Format), fields)
	defer view.close()

	stream := &subscription{
		settings:    settings,
		streams:     streams,
		view:        view,
		limit:       &streamLimit{count: p.Count, deadline: p.Duration},
		parameters:  &opcua.SubscriptionParameters{Interval: p.Interval},
		timestamps:  ua.TimestampsToReturnBoth,
		requests:    requests,
		handles:     []*ua.NodeID{target},
		eventFields: fields,
	}

	return stream.run(ctx)
}

// streamView receives notifications and renders them. Data changes and events
// share the subscription machinery but not their presentation.
type streamView interface {
	// change renders one data change; only the data-change view implements it
	// meaningfully.
	change(node *ua.NodeID, value *ua.DataValue)
	// event renders one event.
	event(fields []*ua.Variant)
	// status renders a connection state change, such as a failover.
	status(message string)
	// label names a monitored node, once its browse name is known.
	label(nodeID, name string)
	// flush renders anything buffered, once per notification batch.
	flush()
	close()
}

// streamLimit stops a stream after a number of notifications or a duration.
type streamLimit struct {
	count    int
	deadline time.Duration
	seen     int
}

func (l *streamLimit) reached() bool {
	return l.count > 0 && l.seen >= l.count
}

// subscription runs a subscription until the context ends, the limit is reached,
// or every endpoint stops answering.
//
// A dropped session is not the end: gopcua restores one transparently where it
// can, and where it cannot this re-dials, which walks the configured failover
// order again and re-creates the subscription on whichever endpoint answers.
// That is what makes a long-running monitor survive a redundant pair switching
// over.
type subscription struct {
	settings    *conn.Settings
	streams     *output
	view        streamView
	limit       *streamLimit
	parameters  *opcua.SubscriptionParameters
	timestamps  ua.TimestampsToReturn
	requests    []*ua.MonitoredItemCreateRequest
	handles     []*ua.NodeID
	eventFields []string
}

func (s *subscription) run(ctx context.Context) error {
	if s.limit.deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.limit.deadline)
		defer cancel()
	}

	backoff := time.Second
	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			return nil
		}
		if attempt > 0 {
			s.view.status(fmt.Sprintf("reconnecting in %s", backoff))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			// Back off up to half a minute: a server that is restarting is
			// usually back within that, and polling faster only adds load.
			backoff = min(backoff*2, 30*time.Second)
		}

		err := s.once(ctx)
		switch {
		case err == nil:
			return nil
		case ctx.Err() != nil:
			return nil
		case errors.Is(err, errLimit):
			return nil
		case !s.settings.Reconnect:
			return err
		}

		s.streams.Err.Warnf("subscription lost: %v", err)
		backoff = time.Second
	}
}

// errLimit unwinds a stream that has delivered everything it was asked for.
var errLimit = errors.New("limit reached")

func (s *subscription) once(ctx context.Context) error {
	client, err := conn.Dial(ctx, s.settings, s.streams.notice(s.settings.Verbosity))
	if err != nil {
		return err
	}
	defer client.CloseQuietly(ctx)

	s.view.status(fmt.Sprintf("subscribed to %s via %s", client.Endpoint, client.Security()))

	notifications := make(chan *opcua.PublishNotificationData, 64)
	sub, err := client.Subscribe(ctx, s.parameters, notifications)
	if err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}
	defer func() {
		// Cancelling needs a live context, and the one passed in is usually
		// already cancelled by the time a stream unwinds.
		cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = sub.Cancel(cancelCtx)
	}()

	response, err := sub.Monitor(ctx, s.timestamps, s.requests...)
	if err != nil {
		return fmt.Errorf("monitor items: %w", err)
	}

	failed := 0
	for index, result := range response.Results {
		if result.StatusCode == ua.StatusOK {
			continue
		}
		failed++
		target := "item"
		if index < len(s.handles) {
			target = s.handles[index].String()
		}
		s.streams.Err.Warnf("%s: %s (%s)", target, opc.StatusName(result.StatusCode),
			opc.StatusDescription(result.StatusCode))
	}
	if failed == len(response.Results) {
		return fmt.Errorf("%w: no node could be monitored", ErrService)
	}

	s.applyLabels(ctx, client)

	for {
		select {
		case <-ctx.Done():
			return nil

		case notification := <-notifications:
			if notification == nil {
				continue
			}
			if notification.Error != nil {
				return notification.Error
			}

			// The limit is enforced per notification rather than per batch: a
			// publish carries every item that changed in one interval, so
			// checking only between batches would overshoot --count by up to
			// the number of monitored nodes.
			done := false
			switch value := notification.Value.(type) {
			case *ua.DataChangeNotification:
				for _, item := range value.MonitoredItems {
					if item == nil {
						continue
					}
					s.view.change(s.nodeFor(item.ClientHandle), item.Value)
					s.limit.seen++
					if s.limit.reached() {
						done = true
						break
					}
				}
			case *ua.EventNotificationList:
				for _, item := range value.Events {
					if item == nil {
						continue
					}
					s.view.event(item.EventFields)
					s.limit.seen++
					if s.limit.reached() {
						done = true
						break
					}
				}
			}

			s.view.flush()
			if done {
				return errLimit
			}
		}
	}
}

// applyLabels names the monitored nodes by their browse name, in one read for
// all of them. A numeric node id says nothing about what it measures, and the
// name is what an operator recognises; failing to read it is not worth reporting,
// since the node id is still shown.
func (s *subscription) applyLabels(ctx context.Context, client *conn.Client) {
	if len(s.handles) == 0 {
		return
	}

	values, err := opc.ReadMany(ctx, client, s.handles, ua.AttributeIDBrowseName)
	if err != nil {
		return
	}

	for index, value := range values {
		if index >= len(s.handles) || value == nil || value.Status != ua.StatusOK {
			continue
		}
		if name := opc.Text(value.Value); name != "" {
			s.view.label(s.handles[index].String(), name)
		}
	}
}

// nodeFor maps a client handle back to the node that was monitored.
func (s *subscription) nodeFor(handle uint32) *ua.NodeID {
	index := int(handle) - 1
	if index < 0 || index >= len(s.handles) {
		return ua.NewTwoByteNodeID(0)
	}
	return s.handles[index]
}

func monitoringMode(name string) ua.MonitoringMode {
	switch name {
	case "sampling":
		return ua.MonitoringModeSampling
	case "disabled":
		return ua.MonitoringModeDisabled
	default:
		return ua.MonitoringModeReporting
	}
}

// dataChangeFilter builds the filter that decides what counts as a change: which
// parts of a value are compared, and how large a move has to be to be reported.
func dataChangeFilter(trigger, deadband string, value float64) (*ua.ExtensionObject, error) {
	var deadbandType uint32
	switch deadband {
	case "", "none":
		deadbandType = 0
	case "absolute":
		deadbandType = 1
	case "percent":
		deadbandType = 2
	default:
		return nil, fmt.Errorf("unknown deadband %q (known: none, absolute, percent)", deadband)
	}

	if deadbandType != 0 && value <= 0 {
		return nil, fmt.Errorf("a %s deadband needs a positive --deadband-value", deadband)
	}
	if deadbandType == 2 && value > 100 {
		return nil, fmt.Errorf("a percent deadband takes 0 to 100, got %g", value)
	}

	var dataChangeTrigger ua.DataChangeTrigger
	switch trigger {
	case "status":
		dataChangeTrigger = ua.DataChangeTriggerStatus
	case "status-value-timestamp":
		dataChangeTrigger = ua.DataChangeTriggerStatusValueTimestamp
	default:
		dataChangeTrigger = ua.DataChangeTriggerStatusValue
	}

	// A subscription with no deadband and the default trigger needs no filter at
	// all, and some servers reject one they consider redundant.
	if deadbandType == 0 && dataChangeTrigger == ua.DataChangeTriggerStatusValue {
		return nil, nil
	}

	return ua.NewExtensionObject(&ua.DataChangeFilter{
		Trigger:       dataChangeTrigger,
		DeadbandType:  deadbandType,
		DeadbandValue: value,
	}), nil
}

// dataView renders data changes. The live format keeps one row per node and
// redraws it; the others append a record per change.
type dataView struct {
	streams *output
	format  string
	stats   bool

	live    *render.Live
	writer  *csv.Writer
	encoder *json.Encoder

	mutex   sync.Mutex
	order   []string
	rows    map[string]*liveRow
	updates int
	started time.Time
	message string
}

// liveRow is the current state of one monitored node in the live view.
type liveRow struct {
	nodeID   string
	name     string
	value    string
	typeName string
	status   string
	stamp    time.Time
	updates  int
	history  []float64
}

func newStreamView(streams *output, format string, targets []*ua.NodeID, stats bool) streamView {
	view := &dataView{
		streams: streams,
		format:  format,
		stats:   stats,
		rows:    map[string]*liveRow{},
		started: time.Now(),
	}

	for _, target := range targets {
		key := target.String()
		view.order = append(view.order, key)
		view.rows[key] = &liveRow{
			nodeID: key,
			name:   shortName(target),
			status: "-",
		}
	}

	switch format {
	case "live":
		view.live = streams.Out.Live()
	case "csv":
		view.writer = csv.NewWriter(streams.Out.Raw())
		_ = view.writer.Write([]string{"timestamp", "nodeId", "value", "type", "status"})
	case "jsonl":
		view.encoder = json.NewEncoder(streams.Out.Raw())
	}

	return view
}

func (v *dataView) change(target *ua.NodeID, value *ua.DataValue) {
	if value == nil {
		return
	}

	v.mutex.Lock()
	defer v.mutex.Unlock()
	v.updates++

	key := target.String()
	stamp := value.SourceTimestamp
	if stamp.IsZero() {
		stamp = value.ServerTimestamp
	}
	if stamp.IsZero() {
		stamp = time.Now()
	}

	switch v.format {
	case "plain":
		fmt.Fprintf(v.streams.Out.Raw(), "%s\t%s\t%s\t%s\n",
			stamp.Local().Format("15:04:05.000"), key, opc.Text(value.Value), opc.StatusName(value.Status))
		return

	case "csv":
		_ = v.writer.Write([]string{
			stamp.UTC().Format(time.RFC3339Nano),
			key,
			opc.Text(value.Value),
			opc.TypeName(value.Value),
			opc.StatusName(value.Status),
		})
		return

	case "jsonl":
		result := cligen.ReadResult{
			NodeID:    key,
			Attribute: "Value",
			Status:    opc.StatusName(value.Status),
			Value:     opc.JSON(value.Value),
			Text:      new(opc.Text(value.Value)),
			Type:      new(opc.TypeName(value.Value)),
		}
		if !value.SourceTimestamp.IsZero() {
			result.SourceTimestamp = &value.SourceTimestamp
		}
		if !value.ServerTimestamp.IsZero() {
			result.ServerTimestamp = &value.ServerTimestamp
		}
		_ = v.encoder.Encode(result)
		return
	}

	row, ok := v.rows[key]
	if !ok {
		row = &liveRow{nodeID: key, name: shortName(target)}
		v.rows[key] = row
		v.order = append(v.order, key)
	}

	row.value = opc.Text(value.Value)
	row.typeName = opc.TypeName(value.Value)
	row.status = opc.StatusName(value.Status)
	row.stamp = stamp
	row.updates++

	// The sparkline only means something for a numeric series, and a bounded
	// window keeps memory flat over a long run.
	if number, ok := opc.Numeric(value.Value); ok {
		row.history = append(row.history, number)
		if len(row.history) > 40 {
			row.history = row.history[len(row.history)-40:]
		}
	}
}

func (v *dataView) event([]*ua.Variant) {}

func (v *dataView) label(nodeID, name string) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if row, ok := v.rows[nodeID]; ok {
		row.name = name
	}
}

func (v *dataView) status(message string) {
	v.mutex.Lock()
	v.message = message
	v.mutex.Unlock()

	if v.format == "live" {
		v.flush()
		return
	}
	if v.streams.Err != nil {
		v.streams.Err.Notef("%s", message)
	}
}

func (v *dataView) flush() {
	switch v.format {
	case "csv":
		v.writer.Flush()
		return
	case "live":
	default:
		return
	}

	v.mutex.Lock()
	defer v.mutex.Unlock()

	p := v.streams.Out
	styles := p.T.Styles

	rows := make([][]string, 0, len(v.order))
	for _, key := range v.order {
		row := v.rows[key]
		spark := ""
		if len(row.history) > 1 {
			spark = styles.Accent.Render(p.Sparkline(row.history))
		}
		stamp := ""
		if !row.stamp.IsZero() {
			stamp = row.stamp.Local().Format("15:04:05.000")
		}

		rows = append(rows, []string{
			row.name,
			render.Truncate(row.value, 28),
			styles.Type.Render(row.typeName),
			statusLabel(p, row.status),
			styles.Faint.Render(stamp),
			spark,
			styles.Faint.Render(strconv.Itoa(row.updates)),
		})
	}

	var frame strings.Builder
	frame.WriteString(styles.Title.Render("● monitoring"))
	frame.WriteString("  ")
	frame.WriteString(styles.Faint.Render(fmt.Sprintf("%d node(s) · %d update(s) · %s",
		len(v.order), v.updates, time.Since(v.started).Round(time.Second))))
	if v.message != "" {
		frame.WriteString("  ")
		frame.WriteString(styles.Dim.Render(v.message))
	}
	frame.WriteString("\n")

	frame.WriteString(liveTable(p,
		[]string{"NODE", "VALUE", "TYPE", "STATUS", "UPDATED", "TREND", "N"}, rows))

	if v.stats {
		frame.WriteString("\n")
		frame.WriteString(styles.Faint.Render(fmt.Sprintf("updates %d · rate %.1f/s",
			v.updates, float64(v.updates)/max(time.Since(v.started).Seconds(), 1))))
	}
	frame.WriteString("\n")
	frame.WriteString(styles.Faint.Render("ctrl-c to stop"))

	v.live.Render(frame.String())
}

func (v *dataView) close() {
	if v.writer != nil {
		v.writer.Flush()
	}
	if v.live != nil {
		v.live.Finish()
	}
}

// eventView renders an event stream as a scrolling log, which is how an operator
// reads alarms: newest at the bottom, severity carrying the colour.
type eventView struct {
	streams *output
	format  string
	fields  []string

	writer  *csv.Writer
	encoder *json.Encoder
	count   int
}

func newEventView(streams *output, format string, fields []string) streamView {
	view := &eventView{streams: streams, format: format, fields: fields}

	switch format {
	case "csv":
		view.writer = csv.NewWriter(streams.Out.Raw())
		header := append([]string{"receivedAt"}, fields...)
		_ = view.writer.Write(header)
	case "jsonl":
		view.encoder = json.NewEncoder(streams.Out.Raw())
	case "live":
		streams.Out.Title("● events", strings.Join(fields, ", "))
	}

	return view
}

func (v *eventView) change(*ua.NodeID, *ua.DataValue) {}

// label has nothing to do for an event stream: an event names its own source.
func (v *eventView) label(string, string) {}

func (v *eventView) event(values []*ua.Variant) {
	event := opc.DescribeEvent(v.fields, values)
	v.count++

	switch v.format {
	case "jsonl":
		_ = v.encoder.Encode(event)

	case "csv":
		row := []string{time.Now().UTC().Format(time.RFC3339Nano)}
		for _, field := range v.fields {
			row = append(row, fmt.Sprint(event.Fields[field]))
		}
		_ = v.writer.Write(row)

	case "plain":
		fmt.Fprintf(v.streams.Out.Raw(), "%s\t%d\t%s\t%s\t%s\n",
			eventTime(event),
			derefOrZero(event.Severity),
			derefOr(event.SourceName, ""),
			derefOr(event.EventType, ""),
			derefOr(event.Message, ""))

	default:
		p := v.streams.Out
		p.Printf("%s %s %s %s\n",
			p.T.Styles.Faint.Render(eventTime(event)),
			severityCell(p, event.Severity),
			p.T.Styles.Subtitle.Render(derefOr(event.SourceName, "-")),
			p.T.Styles.Value.Render(derefOr(event.Message, derefOr(event.EventType, ""))))
	}
}

func (v *eventView) status(message string) {
	if v.format == "live" {
		v.streams.Out.Notef("%s", message)
		return
	}
	v.streams.Err.Notef("%s", message)
}

func (v *eventView) flush() {
	if v.writer != nil {
		v.writer.Flush()
	}
}

func (v *eventView) close() {
	if v.writer != nil {
		v.writer.Flush()
	}
	if v.format == "live" && v.count == 0 {
		v.streams.Err.Warnf("no events arrived; the node may not be an event notifier, or nothing happened")
	}
}

// liveTable renders a compact table for an in-place frame. The lipgloss table
// renderer is not used here because it re-measures every frame and the column
// widths would jitter as values change length.
func liveTable(p *render.Printer, headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for index, header := range headers {
		widths[index] = lipgloss.Width(header)
	}
	for _, row := range rows {
		for index, cell := range row {
			if index < len(widths) && lipgloss.Width(cell) > widths[index] {
				widths[index] = lipgloss.Width(cell)
			}
		}
	}

	var b strings.Builder
	for index, header := range headers {
		b.WriteString(p.T.Styles.Header.Render(pad(header, widths[index])))
	}
	b.WriteString("\n")

	for _, row := range rows {
		for index, cell := range row {
			if index >= len(widths) {
				continue
			}
			b.WriteString(p.T.Styles.Cell.Render(pad(cell, widths[index])))
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// pad right-pads a cell to width, measuring in display cells so styled text and
// wide glyphs line up.
func pad(cell string, width int) string {
	gap := width - lipgloss.Width(cell)
	if gap <= 0 {
		return cell
	}
	return cell + strings.Repeat(" ", gap)
}

// shortName is the display label for a monitored node: its identifier part,
// which is what distinguishes one node of a set from another.
func shortName(target *ua.NodeID) string {
	if name := opc.StandardName(target); name != "" {
		return name
	}
	text := target.String()
	if index := strings.LastIndex(text, ";"); index >= 0 {
		return text[index+1:]
	}
	return text
}

func derefOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
