package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/opc"
	"github.com/Southclaws/opcuacli/internal/render"
)

// historyRead reads archived values over a time range.
func historyRead(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.HistoryReadParams) (cligen.HistoryResult, error) {
	target, err := node(p.Node)
	if err != nil {
		return cligen.HistoryResult{}, err
	}

	start, end, err := timeRange(p.Start, p.End, p.Last)
	if err != nil {
		return cligen.HistoryResult{}, err
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return cligen.HistoryResult{}, err
	}
	defer session.Close(ctx)

	result, err := opc.HistoryRead(ctx, session.Client, opc.HistoryOptions{
		Node:     target,
		Start:    start,
		End:      end,
		Limit:    p.Limit,
		Bounds:   p.Bounds,
		Modified: p.Modified,
	})
	if err != nil {
		return result, fmt.Errorf("%w: %w", ErrService, err)
	}

	return result, renderHistory(session, string(p.Format), result, start, end)
}

// historyAt reads the value at specific points in time.
func historyAt(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.HistoryAtParams) (cligen.HistoryResult, error) {
	target, err := node(p.Node)
	if err != nil {
		return cligen.HistoryResult{}, err
	}

	now := time.Now()
	times := make([]time.Time, 0, len(p.Time))
	for _, text := range p.Time {
		stamp, err := opc.ParseTime(text, now)
		if err != nil {
			return cligen.HistoryResult{}, err
		}
		times = append(times, stamp)
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return cligen.HistoryResult{}, err
	}
	defer session.Close(ctx)

	result, err := opc.HistoryAt(ctx, session.Client, target, times, p.UseSimpleBounds)
	if err != nil {
		return result, fmt.Errorf("%w: %w", ErrService, err)
	}

	return result, renderHistory(session, string(p.Format), result, time.Time{}, time.Time{})
}

func renderHistory(session *session, format string, result cligen.HistoryResult, start, end time.Time) error {
	if machineReadable(format) {
		return nil
	}

	switch format {
	case "jsonl":
		encoder := json.NewEncoder(session.Out.Raw())
		for _, value := range result.Values {
			if err := encoder.Encode(value); err != nil {
				return err
			}
		}

	case "plain":
		rows := make([][]string, 0, len(result.Values))
		for _, value := range result.Values {
			rows = append(rows, []string{
				historyStamp(value),
				derefOr(value.Text, ""),
				value.Status,
			})
		}
		session.Out.Plain(rows)

	default:
		subtitle := fmt.Sprintf("%d value(s)", result.Count)
		if !start.IsZero() && !end.IsZero() {
			subtitle = fmt.Sprintf("%d value(s) from %s to %s",
				result.Count, start.Local().Format("2006-01-02 15:04:05"), end.Local().Format("15:04:05"))
		}
		session.Out.Title(result.NodeID, subtitle)

		if result.Count == 0 {
			session.Err.Warnf("no archived values; the node may not be historizing")
			return nil
		}

		rows := make([][]string, 0, len(result.Values))
		for _, value := range result.Values {
			rows = append(rows, []string{
				historyStamp(value),
				render.Truncate(derefOr(value.Text, ""), 48),
				statusLabel(session.Out, value.Status),
			})
		}
		session.Out.Table([]string{"TIMESTAMP", "VALUE", "STATUS"}, rows)

		if spark := historySparkline(session.Out, result); spark != "" {
			session.Out.Printf("  %s\n", spark)
		}
		if result.Truncated != nil && *result.Truncated {
			session.Err.Warnf("stopped at the --limit of %d; the range holds more", result.Count)
		}
	}

	return nil
}

func historyStamp(value cligen.HistoryValue) string {
	stamp := value.SourceTimestamp
	if stamp == nil {
		stamp = value.ServerTimestamp
	}
	if stamp == nil {
		return ""
	}
	return stamp.Local().Format("2006-01-02 15:04:05.000")
}

// historySparkline draws the shape of a numeric series under the table, which is
// the fastest way to see a trend or a gap in archived data.
func historySparkline(p *render.Printer, result cligen.HistoryResult) string {
	numbers := make([]float64, 0, len(result.Values))
	for _, value := range result.Values {
		number, ok := value.Value.(float64)
		if !ok {
			return ""
		}
		numbers = append(numbers, number)
	}
	if len(numbers) < 2 {
		return ""
	}
	return p.T.Styles.Accent.Render(p.Sparkline(numbers))
}

// historyEvents reads archived events of a notifier node.
func historyEvents(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.HistoryEventsParams) (cligen.EventList, error) {
	target, err := node(p.Node)
	if err != nil {
		return nil, err
	}
	eventType, err := node(p.EventType)
	if err != nil {
		return nil, err
	}

	start, end, err := timeRange(p.Start, p.End, p.Last)
	if err != nil {
		return nil, err
	}

	fields := p.Field
	if len(fields) == 0 {
		fields = opc.DefaultEventFields()
	}

	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return nil, err
	}
	defer session.Close(ctx)

	events, err := opc.HistoryEvents(ctx, session.Client, target, fields, eventType, start, end, p.Limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return events, nil
	}

	switch format {
	case "jsonl":
		encoder := json.NewEncoder(session.Out.Raw())
		for _, event := range events {
			if err := encoder.Encode(event); err != nil {
				return nil, err
			}
		}

	case "plain":
		rows := make([][]string, 0, len(events))
		for _, event := range events {
			rows = append(rows, eventRow(event, fields))
		}
		session.Out.Plain(rows)

	default:
		session.Out.Title(target.String(), fmt.Sprintf("%d event(s)", len(events)))
		if len(events) == 0 {
			session.Err.Warnf("no archived events in the range; the notifier may not historize events")
			return events, nil
		}
		rows := make([][]string, 0, len(events))
		for _, event := range events {
			rows = append(rows, []string{
				eventTime(event),
				severityCell(session.Out, event.Severity),
				derefOr(event.SourceName, ""),
				derefOr(event.EventType, ""),
				render.Truncate(derefOr(event.Message, ""), 60),
			})
		}
		session.Out.Table([]string{"TIME", "SEVERITY", "SOURCE", "TYPE", "MESSAGE"}, rows)
	}

	return events, nil
}

func eventRow(event cligen.Event, fields []string) []string {
	row := make([]string, 0, len(fields)+1)
	row = append(row, eventTime(event))
	for _, field := range fields {
		if field == "Time" {
			continue
		}
		row = append(row, fmt.Sprint(event.Fields[field]))
	}
	return row
}

func eventTime(event cligen.Event) string {
	stamp := event.Time
	if stamp == nil {
		stamp = event.ReceiveTime
	}
	if stamp == nil {
		return ""
	}
	return stamp.Local().Format("2006-01-02 15:04:05.000")
}

// severityCell colours an event severity by the band it falls in, so a critical
// alarm is visible in a long list.
func severityCell(p *render.Printer, severity *int) string {
	if severity == nil {
		return ""
	}
	text := fmt.Sprintf("%d %s", *severity, opc.SeverityLabel(*severity))
	switch {
	case *severity >= 700:
		return p.T.Styles.Bad.Render(text)
	case *severity >= 500:
		return p.T.Styles.Uncertain.Render(text)
	default:
		return p.T.Styles.Dim.Render(text)
	}
}

// timeRange resolves the --start, --end and --last flags into an absolute range.
// An explicit --start wins over --last, and an absent --end means now.
func timeRange(startText, endText string, last time.Duration) (time.Time, time.Time, error) {
	now := time.Now()

	end := now
	if endText != "" {
		parsed, err := opc.ParseTime(endText, now)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--end: %w", err)
		}
		end = parsed
	}

	if startText != "" {
		start, err := opc.ParseTime(startText, now)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--start: %w", err)
		}
		if !start.Before(end) {
			return time.Time{}, time.Time{}, fmt.Errorf("the --start time %s is not before the --end time %s",
				start.Format(time.RFC3339), end.Format(time.RFC3339))
		}
		return start, end, nil
	}

	if last <= 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("a time range needs a positive --last, or a --start")
	}
	return end.Add(-last), end, nil
}

// severityFilter validates the --severity-min flag, whose useful range the
// specification fixes at 1 to 1000.
func severityFilter(minimum int) (uint16, error) {
	if minimum < 0 || minimum > 1000 {
		return 0, fmt.Errorf("the --severity-min value must be between 0 and 1000, got %d", minimum)
	}
	return uint16(minimum), nil
}
