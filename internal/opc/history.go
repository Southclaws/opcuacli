package opc

import (
	"context"
	"fmt"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// Historian is the subset of the client needed to read history.
type Historian interface {
	Reader
	HistoryReadRawModified(context.Context, []*ua.HistoryReadValueID, *ua.ReadRawModifiedDetails) (*ua.HistoryReadResponse, error)
	HistoryReadAtTime(context.Context, []*ua.HistoryReadValueID, *ua.ReadAtTimeDetails) (*ua.HistoryReadResponse, error)
	HistoryReadEvent(context.Context, []*ua.HistoryReadValueID, *ua.ReadEventDetails) (*ua.HistoryReadResponse, error)
}

// HistoryOptions controls a raw history read.
type HistoryOptions struct {
	Node     *ua.NodeID
	Start    time.Time
	End      time.Time
	Limit    int
	Bounds   bool
	Modified bool
}

// HistoryRead reads archived values over a time range, following continuation
// points until the range is covered or the limit is reached.
//
// A server returns history in pages and hands back a continuation point for the
// rest; abandoning one without releasing it leaves state on the server, so the
// final call releases it explicitly.
func HistoryRead(ctx context.Context, client Historian, options HistoryOptions) (cligen.HistoryResult, error) {
	result := cligen.HistoryResult{NodeID: options.Node.String()}
	if !options.Start.IsZero() {
		result.Start = &options.Start
	}
	if !options.End.IsZero() {
		result.End = &options.End
	}

	perCall := uint32(0)
	if options.Limit > 0 {
		perCall = uint32(options.Limit)
	}

	var continuation []byte
	for {
		details := &ua.ReadRawModifiedDetails{
			IsReadModified:   options.Modified,
			StartTime:        options.Start,
			EndTime:          options.End,
			NumValuesPerNode: perCall,
			ReturnBounds:     options.Bounds,
		}

		response, err := client.HistoryReadRawModified(ctx, []*ua.HistoryReadValueID{{
			NodeID:            options.Node,
			ContinuationPoint: continuation,
			DataEncoding:      &ua.QualifiedName{},
		}}, details)
		if err != nil {
			return result, serviceError(err)
		}
		if err := serviceResult(response.ResponseHeader); err != nil {
			return result, err
		}
		if len(response.Results) == 0 || response.Results[0] == nil {
			break
		}

		page := response.Results[0]
		if page.StatusCode != ua.StatusOK {
			return result, fmt.Errorf("history read %s: %s (%s)",
				options.Node, StatusName(page.StatusCode), StatusDescription(page.StatusCode))
		}

		data, ok := historyData(page.HistoryData)
		if !ok {
			break
		}
		for _, value := range data.DataValues {
			if options.Limit > 0 && len(result.Values) >= options.Limit {
				result.Truncated = new(true)
				break
			}
			result.Values = append(result.Values, historyValue(value))
		}

		continuation = page.ContinuationPoint
		done := len(continuation) == 0 || (options.Limit > 0 && len(result.Values) >= options.Limit)
		if !done {
			continue
		}
		if len(continuation) > 0 {
			releaseContinuation(ctx, client, options.Node, continuation)
		}
		break
	}

	result.Count = len(result.Values)
	return result, nil
}

// HistoryAt reads the value of a node at specific points in time, which the
// server answers by interpolating or by taking the bounding value.
func HistoryAt(ctx context.Context, client Historian, node *ua.NodeID, times []time.Time, simpleBounds bool) (cligen.HistoryResult, error) {
	result := cligen.HistoryResult{NodeID: node.String()}
	if len(times) == 0 {
		return result, fmt.Errorf("reading at a point in time needs at least one --time")
	}

	response, err := client.HistoryReadAtTime(ctx, []*ua.HistoryReadValueID{{
		NodeID:       node,
		DataEncoding: &ua.QualifiedName{},
	}}, &ua.ReadAtTimeDetails{
		ReqTimes:        times,
		UseSimpleBounds: simpleBounds,
	})
	if err != nil {
		return result, serviceError(err)
	}
	if err := serviceResult(response.ResponseHeader); err != nil {
		return result, err
	}
	if len(response.Results) == 0 || response.Results[0] == nil {
		return result, nil
	}

	page := response.Results[0]
	if page.StatusCode != ua.StatusOK {
		return result, fmt.Errorf("history read at time %s: %s (%s)",
			node, StatusName(page.StatusCode), StatusDescription(page.StatusCode))
	}

	if data, ok := historyData(page.HistoryData); ok {
		for _, value := range data.DataValues {
			result.Values = append(result.Values, historyValue(value))
		}
	}

	first, last := times[0], times[len(times)-1]
	result.Start, result.End = &first, &last
	result.Count = len(result.Values)
	return result, nil
}

// HistoryEvents reads archived events of a notifier node.
//
// The select clauses decide which fields come back, and in what order; the
// returned events are keyed by the same field names so a caller can read them
// without tracking positions.
func HistoryEvents(ctx context.Context, client Historian, node *ua.NodeID, fields []string, eventType *ua.NodeID, start, end time.Time, limit int) ([]cligen.Event, error) {
	if len(fields) == 0 {
		fields = DefaultEventFields()
	}
	if eventType == nil {
		eventType = ua.NewNumericNodeID(0, id.BaseEventType)
	}

	response, err := client.HistoryReadEvent(ctx, []*ua.HistoryReadValueID{{
		NodeID:       node,
		DataEncoding: &ua.QualifiedName{},
	}}, &ua.ReadEventDetails{
		NumValuesPerNode: uint32(max(limit, 0)),
		StartTime:        start,
		EndTime:          end,
		Filter:           EventFilter(fields, eventType, 0),
	})
	if err != nil {
		return nil, serviceError(err)
	}
	if err := serviceResult(response.ResponseHeader); err != nil {
		return nil, err
	}
	if len(response.Results) == 0 || response.Results[0] == nil {
		return nil, nil
	}

	page := response.Results[0]
	if page.StatusCode != ua.StatusOK {
		return nil, fmt.Errorf("history read events %s: %s (%s)",
			node, StatusName(page.StatusCode), StatusDescription(page.StatusCode))
	}

	history, ok := page.HistoryData.Value.(*ua.HistoryEvent)
	if !ok || history == nil {
		return nil, nil
	}

	events := make([]cligen.Event, 0, len(history.Events))
	for _, entry := range history.Events {
		if entry == nil {
			continue
		}
		if limit > 0 && len(events) >= limit {
			break
		}
		events = append(events, DescribeEvent(fields, entry.EventFields))
	}

	return events, nil
}

// releaseContinuation tells the server the rest of a history page is not wanted,
// so it can drop the state it was holding for it.
func releaseContinuation(ctx context.Context, client Historian, node *ua.NodeID, continuation []byte) {
	_, _ = client.HistoryReadRawModified(ctx, []*ua.HistoryReadValueID{{
		NodeID:            node,
		ContinuationPoint: continuation,
		DataEncoding:      &ua.QualifiedName{},
	}}, &ua.ReadRawModifiedDetails{})
}

// historyData unwraps the extension object a history result carries.
func historyData(object *ua.ExtensionObject) (*ua.HistoryData, bool) {
	if object == nil {
		return nil, false
	}
	data, ok := object.Value.(*ua.HistoryData)
	return data, ok && data != nil
}

func historyValue(value *ua.DataValue) cligen.HistoryValue {
	if value == nil {
		return cligen.HistoryValue{Status: StatusName(ua.StatusBadNoData)}
	}

	entry := cligen.HistoryValue{Status: StatusName(value.Status)}
	if !value.SourceTimestamp.IsZero() {
		entry.SourceTimestamp = &value.SourceTimestamp
	}
	if !value.ServerTimestamp.IsZero() {
		entry.ServerTimestamp = &value.ServerTimestamp
	}
	if value.Value != nil {
		entry.Value = JSON(value.Value)
		entry.Text = new(Text(value.Value))
	}
	return entry
}
