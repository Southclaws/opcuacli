package opc

import (
	"context"
	"fmt"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// ServerInfo reads the server's status: the run state, when it started, its own
// clock, the build it is running, and the service level a redundant pair uses to
// advertise which member to prefer.
//
// ServerStatus is one structure but is read field by field here, because a
// server that does not publish the composite value usually still publishes the
// components, and a partial answer is more useful than a failure.
func ServerInfo(ctx context.Context, client Reader, endpoint string) (cligen.ServerInfo, error) {
	fields := []struct {
		node   uint32
		assign func(*cligen.ServerInfo, *ua.Variant)
	}{
		{id.Server_ServerStatus_State, func(info *cligen.ServerInfo, value *ua.Variant) {
			info.State = serverStateName(ua.ServerState(value.Int()))
		}},
		{id.Server_ServerStatus_StartTime, func(info *cligen.ServerInfo, value *ua.Variant) {
			if stamp := value.Time(); !stamp.IsZero() {
				info.StartTime = &stamp
			}
		}},
		{id.Server_ServerStatus_CurrentTime, func(info *cligen.ServerInfo, value *ua.Variant) {
			if stamp := value.Time(); !stamp.IsZero() {
				info.CurrentTime = &stamp
			}
		}},
		{id.Server_ServerStatus_SecondsTillShutdown, func(info *cligen.ServerInfo, value *ua.Variant) {
			seconds := int(value.Int())
			info.SecondsTillShutdown = &seconds
		}},
		{id.Server_ServerStatus_ShutdownReason, func(info *cligen.ServerInfo, value *ua.Variant) {
			if text := Text(value); text != "" {
				info.ShutdownReason = &text
			}
		}},
		{id.Server_ServerStatus_BuildInfo_ProductName, func(info *cligen.ServerInfo, value *ua.Variant) {
			info.ProductName = textOrNil(value)
		}},
		{id.Server_ServerStatus_BuildInfo_ProductURI, func(info *cligen.ServerInfo, value *ua.Variant) {
			info.ProductURI = textOrNil(value)
		}},
		{id.Server_ServerStatus_BuildInfo_ManufacturerName, func(info *cligen.ServerInfo, value *ua.Variant) {
			info.ManufacturerName = textOrNil(value)
		}},
		{id.Server_ServerStatus_BuildInfo_SoftwareVersion, func(info *cligen.ServerInfo, value *ua.Variant) {
			info.SoftwareVersion = textOrNil(value)
		}},
		{id.Server_ServerStatus_BuildInfo_BuildNumber, func(info *cligen.ServerInfo, value *ua.Variant) {
			info.BuildNumber = textOrNil(value)
		}},
		{id.Server_ServerStatus_BuildInfo_BuildDate, func(info *cligen.ServerInfo, value *ua.Variant) {
			if stamp := value.Time(); !stamp.IsZero() {
				info.BuildDate = &stamp
			}
		}},
		{id.Server_ServiceLevel, func(info *cligen.ServerInfo, value *ua.Variant) {
			level := int(value.Int())
			info.ServiceLevel = &level
		}},
	}

	nodes := make([]*ua.NodeID, 0, len(fields))
	for _, field := range fields {
		nodes = append(nodes, ua.NewNumericNodeID(0, field.node))
	}

	values, err := ReadMany(ctx, client, nodes, ua.AttributeIDValue)
	if err != nil {
		return cligen.ServerInfo{}, err
	}

	info := cligen.ServerInfo{Endpoint: endpoint, State: "Unknown"}
	for index, field := range fields {
		if index >= len(values) {
			break
		}
		value := values[index]
		if value == nil || value.Status != ua.StatusOK || value.Value == nil {
			continue
		}
		field.assign(&info, value.Value)
	}

	if info.StartTime != nil && info.CurrentTime != nil {
		uptime := info.CurrentTime.Sub(*info.StartTime).Round(time.Second)
		info.Uptime = new(uptime.String())
	}
	if info.CurrentTime != nil {
		skew := time.Since(*info.CurrentTime).Round(time.Millisecond)
		// The sign reads from the server's point of view: a positive skew means
		// the server clock is ahead of this machine's.
		info.ClockSkew = new((-skew).String())
	}
	if namespaces, err := Namespaces(ctx, client); err == nil {
		info.Namespaces = namespaces
	}

	return info, nil
}

// Capabilities reads the operational limits and conformance profiles a server
// publishes. They are worth checking before a bulk operation: exceeding a limit
// earns BadTooManyOperations rather than a partial result.
func Capabilities(ctx context.Context, client Reader, endpoint string) (cligen.Capabilities, error) {
	capabilities := cligen.Capabilities{Endpoint: endpoint}

	counters := []struct {
		node   uint32
		assign func(int)
	}{
		{id.Server_ServerCapabilities_MaxBrowseContinuationPoints, func(v int) { capabilities.MaxBrowseContinuationPoints = &v }},
		{id.Server_ServerCapabilities_MaxQueryContinuationPoints, func(v int) { capabilities.MaxQueryContinuationPoints = &v }},
		{id.Server_ServerCapabilities_MaxHistoryContinuationPoints, func(v int) { capabilities.MaxHistoryContinuationPoints = &v }},
		{id.Server_ServerCapabilities_MaxArrayLength, func(v int) { capabilities.MaxArrayLength = &v }},
		{id.Server_ServerCapabilities_MaxStringLength, func(v int) { capabilities.MaxStringLength = &v }},
		{id.Server_ServerCapabilities_MaxByteStringLength, func(v int) { capabilities.MaxByteStringLength = &v }},
		{id.Server_ServerCapabilities_OperationLimits_MaxNodesPerRead, func(v int) { capabilities.MaxNodesPerRead = &v }},
		{id.Server_ServerCapabilities_OperationLimits_MaxNodesPerWrite, func(v int) { capabilities.MaxNodesPerWrite = &v }},
		{id.Server_ServerCapabilities_OperationLimits_MaxNodesPerBrowse, func(v int) { capabilities.MaxNodesPerBrowse = &v }},
		{id.Server_ServerCapabilities_OperationLimits_MaxNodesPerMethodCall, func(v int) { capabilities.MaxNodesPerMethodCall = &v }},
		{id.Server_ServerCapabilities_OperationLimits_MaxNodesPerRegisterNodes, func(v int) { capabilities.MaxNodesPerRegisterNodes = &v }},
		{id.Server_ServerCapabilities_OperationLimits_MaxNodesPerTranslateBrowsePathsToNodeIDs, func(v int) { capabilities.MaxNodesPerTranslate = &v }},
		{id.Server_ServerCapabilities_OperationLimits_MaxNodesPerNodeManagement, func(v int) { capabilities.MaxNodesPerNodeManagement = &v }},
		{id.Server_ServerCapabilities_OperationLimits_MaxNodesPerHistoryReadData, func(v int) { capabilities.MaxNodesPerHistoryReadData = &v }},
		{id.Server_ServerCapabilities_OperationLimits_MaxNodesPerHistoryReadEvents, func(v int) { capabilities.MaxNodesPerHistoryReadEvents = &v }},
		{id.Server_ServerCapabilities_OperationLimits_MaxNodesPerHistoryUpdateData, func(v int) { capabilities.MaxNodesPerHistoryUpdateData = &v }},
		{id.Server_ServerCapabilities_OperationLimits_MaxMonitoredItemsPerCall, func(v int) { capabilities.MaxMonitoredItemsPerCall = &v }},
	}

	nodes := make([]*ua.NodeID, 0, len(counters)+3)
	for _, counter := range counters {
		nodes = append(nodes, ua.NewNumericNodeID(0, counter.node))
	}
	nodes = append(nodes,
		ua.NewNumericNodeID(0, id.Server_ServerCapabilities_MinSupportedSampleRate),
		ua.NewNumericNodeID(0, id.Server_ServerCapabilities_ServerProfileArray),
		ua.NewNumericNodeID(0, id.Server_ServerCapabilities_LocaleIDArray),
	)

	values, err := ReadMany(ctx, client, nodes, ua.AttributeIDValue)
	if err != nil {
		return capabilities, err
	}

	for index, counter := range counters {
		if index >= len(values) {
			break
		}
		if value := values[index]; usable(value) {
			counter.assign(int(value.Value.Int()))
		}
	}

	if base := len(counters); base+2 < len(values) {
		if value := values[base]; usable(value) {
			rate := value.Value.Float()
			capabilities.MinSupportedSampleRate = &rate
		}
		if value := values[base+1]; usable(value) {
			capabilities.ServerProfiles = stringList(value.Value)
		}
		if value := values[base+2]; usable(value) {
			capabilities.LocaleIds = stringList(value.Value)
		}
	}

	return capabilities, nil
}

// Diagnostics reads the server's diagnostic counters.
//
// Diagnostics are optional and often switched off, in which case the counters
// read as bad rather than zero; that is reported as not enabled rather than as a
// failure, because it is a configuration choice and not a fault.
func Diagnostics(ctx context.Context, client Reader, endpoint string, withSessions bool) (cligen.Diagnostics, error) {
	diagnostics := cligen.Diagnostics{Endpoint: endpoint}

	nodes := []*ua.NodeID{
		ua.NewNumericNodeID(0, id.Server_ServerDiagnostics_EnabledFlag),
		ua.NewNumericNodeID(0, id.Server_ServerDiagnostics_ServerDiagnosticsSummary),
	}
	values, err := ReadMany(ctx, client, nodes, ua.AttributeIDValue)
	if err != nil {
		return diagnostics, err
	}

	if len(values) > 0 && usable(values[0]) {
		diagnostics.Enabled = values[0].Value.Bool()
	}

	if len(values) > 1 && usable(values[1]) {
		summary := summaryFields(values[1].Value)
		diagnostics.Summary = summary
		for key, target := range map[string]**int{
			"CurrentSessionCount":           &diagnostics.SessionCount,
			"CurrentSubscriptionCount":      &diagnostics.SubscriptionCount,
			"RejectedRequestsCount":         &diagnostics.RejectedRequestsCount,
			"SecurityRejectedRequestsCount": &diagnostics.SecurityRejectedRequestsCount,
		} {
			if raw, ok := summary[key]; ok {
				if count, ok := raw.(uint32); ok {
					value := int(count)
					*target = &value
				}
			}
		}
	}

	if withSessions {
		sessions, err := ReadOne(ctx, client,
			ua.NewNumericNodeID(0, id.Server_ServerDiagnostics_SessionsDiagnosticsSummary_SessionDiagnosticsArray),
			ua.AttributeIDValue)
		if err == nil {
			diagnostics.Sessions = structureList(sessions)
		}
	}

	return diagnostics, nil
}

// Redundancy reads the server's redundancy support and the other members of its
// set.
//
// A cold or warm set expects the client to fail over on its own, which is what a
// profile's endpoint list is for; a transparent set does it server-side and the
// client sees one address.
func Redundancy(ctx context.Context, client Reader, endpoint string, configured []string) (cligen.Redundancy, error) {
	redundancy := cligen.Redundancy{
		Endpoint:            endpoint,
		Support:             "Unknown",
		ConfiguredEndpoints: configured,
	}

	values, err := ReadMany(ctx, client, []*ua.NodeID{
		ua.NewNumericNodeID(0, id.Server_ServerRedundancy_RedundancySupport),
		ua.NewNumericNodeID(0, id.Server_ServerRedundancy_ServerURIArray),
		ua.NewNumericNodeID(0, id.Server_ServiceLevel),
	}, ua.AttributeIDValue)
	if err != nil {
		return redundancy, err
	}

	if len(values) > 0 && usable(values[0]) {
		support := ua.RedundancySupport(values[0].Value.Int())
		redundancy.Support = redundancyName(support)
		transparent := support == ua.RedundancySupportTransparent
		redundancy.Transparent = &transparent
	}
	if len(values) > 1 && usable(values[1]) {
		redundancy.ServerUris = stringList(values[1].Value)
	}
	if len(values) > 2 && usable(values[2]) {
		level := int(values[2].Value.Int())
		redundancy.ServiceLevel = &level
	}

	return redundancy, nil
}

// usable reports whether a data value carries something worth reading.
func usable(value *ua.DataValue) bool {
	return value != nil && value.Status == ua.StatusOK && value.Value != nil
}

func textOrNil(value *ua.Variant) *string {
	text := Text(value)
	if text == "" {
		return nil
	}
	return &text
}

func stringList(value *ua.Variant) []string {
	if value == nil {
		return nil
	}
	if list, ok := value.Value().([]string); ok {
		return list
	}
	if single, ok := value.Value().(string); ok && single != "" {
		return []string{single}
	}
	return nil
}

// summaryFields flattens a diagnostics summary structure into a map, so the
// output carries every counter a server publishes rather than a fixed few.
func summaryFields(value *ua.Variant) map[string]any {
	object, ok := value.Value().(*ua.ExtensionObject)
	if !ok || object == nil {
		return nil
	}
	summary, ok := object.Value.(*ua.ServerDiagnosticsSummaryDataType)
	if !ok || summary == nil {
		return nil
	}

	return map[string]any{
		"ServerViewCount":               summary.ServerViewCount,
		"CurrentSessionCount":           summary.CurrentSessionCount,
		"CumulatedSessionCount":         summary.CumulatedSessionCount,
		"SecurityRejectedSessionCount":  summary.SecurityRejectedSessionCount,
		"RejectedSessionCount":          summary.RejectedSessionCount,
		"SessionTimeoutCount":           summary.SessionTimeoutCount,
		"SessionAbortCount":             summary.SessionAbortCount,
		"CurrentSubscriptionCount":      summary.CurrentSubscriptionCount,
		"CumulatedSubscriptionCount":    summary.CumulatedSubscriptionCount,
		"PublishingIntervalCount":       summary.PublishingIntervalCount,
		"SecurityRejectedRequestsCount": summary.SecurityRejectedRequestsCount,
		"RejectedRequestsCount":         summary.RejectedRequestsCount,
	}
}

// structureList renders an array of diagnostic structures as maps. The
// structures are only decodable when the server publishes their definitions, so
// an undecoded entry is reported by its type rather than dropped.
func structureList(value *ua.Variant) []map[string]any {
	objects, ok := value.Value().([]*ua.ExtensionObject)
	if !ok {
		return nil
	}

	entries := make([]map[string]any, 0, len(objects))
	for _, object := range objects {
		if object == nil {
			continue
		}
		if session, ok := object.Value.(*ua.SessionDiagnosticsDataType); ok && session != nil {
			entries = append(entries, map[string]any{
				"sessionId":                     nodeText(session.SessionID),
				"sessionName":                   session.SessionName,
				"clientDescription":             applicationName(session.ClientDescription),
				"serverUri":                     session.ServerURI,
				"endpointUrl":                   session.EndpointURL,
				"actualSessionTimeout":          session.ActualSessionTimeout,
				"clientConnectionTime":          session.ClientConnectionTime,
				"clientLastContactTime":         session.ClientLastContactTime,
				"currentSubscriptionsCount":     session.CurrentSubscriptionsCount,
				"currentMonitoredItemsCount":    session.CurrentMonitoredItemsCount,
				"currentPublishRequestsInQueue": session.CurrentPublishRequestsInQueue,
			})
			continue
		}
		entries = append(entries, map[string]any{"structure": extensionText(object)})
	}

	return entries
}

func nodeText(node *ua.NodeID) string {
	if node == nil {
		return ""
	}
	return node.String()
}

func applicationName(description *ua.ApplicationDescription) string {
	if description == nil || description.ApplicationName == nil {
		return ""
	}
	return description.ApplicationName.Text
}

// serverStateName names the run state of a server.
func serverStateName(state ua.ServerState) string {
	name := state.String()
	if trimmed := trimPrefix(name, "ServerState"); trimmed != "" {
		return trimmed
	}
	return fmt.Sprintf("Unknown(%d)", state)
}

// redundancyName names a redundancy support mode.
func redundancyName(support ua.RedundancySupport) string {
	name := support.String()
	if trimmed := trimPrefix(name, "RedundancySupport"); trimmed != "" {
		return trimmed
	}
	return fmt.Sprintf("Unknown(%d)", support)
}

func trimPrefix(text, prefix string) string {
	if len(text) > len(prefix) && text[:len(prefix)] == prefix {
		return text[len(prefix):]
	}
	return ""
}
