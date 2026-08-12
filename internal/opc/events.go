package opc

import (
	"strings"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// DefaultEventFields are the fields of the base event type, which every event
// has. They are the useful default for a subscription or a history read: who
// raised it, how badly, when, and what it said.
func DefaultEventFields() []string {
	return []string{"EventId", "EventType", "SourceName", "Time", "Severity", "Message"}
}

// EventFilter builds the filter an event subscription or history read needs: the
// select clauses that decide which fields come back, and a where clause that
// keeps the server from sending events below a severity.
//
// Filtering on the server matters for events: an alarm-heavy server can produce
// thousands a second, and a client-side filter would still pay for all of them.
func EventFilter(fields []string, eventType *ua.NodeID, minimumSeverity uint16) *ua.EventFilter {
	if eventType == nil {
		eventType = ua.NewNumericNodeID(0, id.BaseEventType)
	}

	selects := make([]*ua.SimpleAttributeOperand, 0, len(fields))
	for _, field := range fields {
		selects = append(selects, &ua.SimpleAttributeOperand{
			TypeDefinitionID: eventType,
			BrowsePath:       fieldPath(field),
			AttributeID:      ua.AttributeIDValue,
		})
	}

	filter := &ua.EventFilter{SelectClauses: selects}
	if minimumSeverity == 0 {
		return filter
	}

	filter.WhereClause = &ua.ContentFilter{
		Elements: []*ua.ContentFilterElement{{
			FilterOperator: ua.FilterOperatorGreaterThanOrEqual,
			FilterOperands: []*ua.ExtensionObject{
				{
					EncodingMask: ua.ExtensionObjectBinary,
					TypeID: &ua.ExpandedNodeID{
						NodeID: ua.NewNumericNodeID(0, id.SimpleAttributeOperand_Encoding_DefaultBinary),
					},
					Value: ua.SimpleAttributeOperand{
						TypeDefinitionID: ua.NewNumericNodeID(0, id.BaseEventType),
						BrowsePath:       []*ua.QualifiedName{{Name: "Severity"}},
						AttributeID:      ua.AttributeIDValue,
					},
				},
				{
					EncodingMask: ua.ExtensionObjectBinary,
					TypeID: &ua.ExpandedNodeID{
						NodeID: ua.NewNumericNodeID(0, id.LiteralOperand_Encoding_DefaultBinary),
					},
					Value: ua.LiteralOperand{Value: ua.MustVariant(minimumSeverity)},
				},
			},
		}},
	}

	return filter
}

// EventFilterObject wraps an event filter for a monitored item's monitoring
// parameters, which carry their filter as an extension object.
func EventFilterObject(filter *ua.EventFilter) *ua.ExtensionObject {
	return &ua.ExtensionObject{
		EncodingMask: ua.ExtensionObjectBinary,
		TypeID: &ua.ExpandedNodeID{
			NodeID: ua.NewNumericNodeID(0, id.EventFilter_Encoding_DefaultBinary),
		},
		Value: *filter,
	}
}

// fieldPath turns a field name into a browse path. A nested field is written
// with dots, as in "ConditionName" or "EnabledState.Id", which is how the
// alarm and condition model addresses its sub-fields.
func fieldPath(field string) []*ua.QualifiedName {
	parts := strings.Split(field, ".")
	path := make([]*ua.QualifiedName, 0, len(parts))
	for _, part := range parts {
		namespace, name := splitQualified(part)
		path = append(path, &ua.QualifiedName{NamespaceIndex: namespace, Name: name})
	}
	return path
}

// DescribeEvent pairs the values of an event notification with the field names
// that were selected, and lifts the well-known fields of the base event type
// into their own places so output can rely on them.
func DescribeEvent(fields []string, values []*ua.Variant) cligen.Event {
	event := cligen.Event{Fields: map[string]any{}}

	for index, field := range fields {
		if index >= len(values) {
			break
		}
		value := values[index]
		event.Fields[field] = JSON(value)

		switch field {
		case "EventId":
			if text := Text(value); text != "" {
				event.EventID = &text
			}
		case "EventType":
			if node := value.NodeID(); node != nil {
				event.EventType = new(typeNodeName(node))
			}
		case "SourceName":
			if text := Text(value); text != "" {
				event.SourceName = &text
			}
		case "Severity":
			severity := int(value.Int())
			event.Severity = &severity
		case "Message":
			if text := Text(value); text != "" {
				event.Message = &text
			}
		case "Time":
			if stamp := value.Time(); !stamp.IsZero() {
				event.Time = &stamp
			}
		case "ReceiveTime":
			if stamp := value.Time(); !stamp.IsZero() {
				event.ReceiveTime = &stamp
			}
		}
	}

	// A subscription's own arrival time is the only timestamp available when the
	// server did not select Time, and an event with no time at all is hard to
	// read in a stream.
	if event.Time == nil {
		now := time.Now()
		event.ReceiveTime = &now
	}

	return event
}

// SeverityLabel names an event severity band the way operators speak about them.
// The specification defines 1 to 1000 with no names, but the bands are
// conventional and a number alone is hard to scan in a live stream.
func SeverityLabel(severity int) string {
	switch {
	case severity >= 900:
		return "critical"
	case severity >= 700:
		return "high"
	case severity >= 500:
		return "medium"
	case severity >= 300:
		return "low"
	case severity > 0:
		return "info"
	default:
		return ""
	}
}
