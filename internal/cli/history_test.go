package cli

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestTimeRange(t *testing.T) {
	for _, test := range []struct {
		name  string
		start string
		end   string
		last  time.Duration
		check func(start, end time.Time) bool
	}{
		{
			name: "last is a window ending now",
			last: 2 * time.Hour,
			check: func(start, end time.Time) bool {
				return end.Sub(start) == 2*time.Hour && time.Since(end) < time.Minute
			},
		},
		{
			name:  "an explicit start overrides last",
			start: "-30m",
			last:  24 * time.Hour,
			check: func(start, end time.Time) bool {
				return end.Sub(start).Round(time.Minute) == 30*time.Minute
			},
		},
		{
			name:  "an explicit end moves the window",
			start: "2024-01-01T00:00:00Z",
			end:   "2024-01-02T00:00:00Z",
			check: func(start, end time.Time) bool {
				return end.Sub(start) == 24*time.Hour && start.Year() == 2024
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			start, end, err := timeRange(test.start, test.end, test.last)
			if err != nil {
				t.Fatalf("timeRange: %v", err)
			}
			if !test.check(start, end) {
				t.Errorf("timeRange(%q, %q, %s) = %s to %s", test.start, test.end, test.last, start, end)
			}
		})
	}
}

func TestTimeRangeRejectsAnImpossibleWindow(t *testing.T) {
	for _, test := range []struct {
		name  string
		start string
		end   string
		last  time.Duration
	}{
		{"no window at all", "", "", 0},
		{"a negative window", "", "", -time.Hour},
		{"start after end", "2024-02-01T00:00:00Z", "2024-01-01T00:00:00Z", 0},
		{"an unparseable start", "whenever", "", 0},
		{"an unparseable end", "", "whenever", time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := timeRange(test.start, test.end, test.last); err == nil {
				t.Errorf("timeRange(%q, %q, %s) succeeded, want an error", test.start, test.end, test.last)
			}
		})
	}
}

func TestSeverityFilter(t *testing.T) {
	for _, value := range []int{0, 1, 500, 1000} {
		if _, err := severityFilter(value); err != nil {
			t.Errorf("severityFilter(%d): %v", value, err)
		}
	}
	for _, value := range []int{-1, 1001} {
		if _, err := severityFilter(value); err == nil {
			t.Errorf("severityFilter(%d) succeeded, want an error", value)
		}
	}
}

func TestDataChangeFilter(t *testing.T) {
	// The default trigger with no deadband needs no filter at all, and some
	// servers reject one they consider redundant.
	filter, err := dataChangeFilter("status-value", "none", 0)
	if err != nil {
		t.Fatalf("dataChangeFilter: %v", err)
	}
	if filter != nil {
		t.Error("the default settings produced a filter, want none")
	}

	filter, err = dataChangeFilter("status-value", "absolute", 0.5)
	if err != nil {
		t.Fatalf("dataChangeFilter: %v", err)
	}
	if filter == nil {
		t.Error("a deadband produced no filter")
	}

	filter, err = dataChangeFilter("status", "none", 0)
	if err != nil {
		t.Fatalf("dataChangeFilter: %v", err)
	}
	if filter == nil {
		t.Error("a non-default trigger produced no filter")
	}
}

func TestDataChangeFilterRejectsAnUnusableDeadband(t *testing.T) {
	for _, test := range []struct {
		name     string
		deadband string
		value    float64
	}{
		{"an unknown kind", "sometimes", 1},
		{"absolute with no value", "absolute", 0},
		{"percent over 100", "percent", 150},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := dataChangeFilter("status-value", test.deadband, test.value); err == nil {
				t.Errorf("dataChangeFilter(%q, %g) succeeded, want an error", test.deadband, test.value)
			}
		})
	}
}

func TestMachineReadableFormats(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		if !machineReadable(format) {
			t.Errorf("%q should be encoded by the generated command", format)
		}
	}
	for _, format := range []string{"table", "plain", "tree", "text", "markdown", "jsonl", "csv", "live"} {
		if machineReadable(format) {
			t.Errorf("%q should be rendered by the handler", format)
		}
	}
}

func TestReadLinesTakesTheFirstFieldOfEachLine(t *testing.T) {
	input := "ns=1;i=1\tVariable\tTemperature\n\n# a comment\nns=1;i=2\n"

	lines, err := readLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readLines: %v", err)
	}
	if len(lines) != 2 || lines[0] != "ns=1;i=1" || lines[1] != "ns=1;i=2" {
		t.Errorf("readLines = %v, want the two node ids", lines)
	}
}

func TestTailTruncateKeepsTheEndOfAPath(t *testing.T) {
	path := "Objects/urn:southclaws:factory/COOK_KTL_01/COOK_KTL_01_TEMP_C"

	got := tailTruncate(path, 20)
	if width := lipgloss.Width(got); width > 20 {
		t.Errorf("tailTruncate returned %d cells, want at most 20", width)
	}
	if !hasSuffix(got, "COOK_KTL_01_TEMP_C") {
		t.Errorf("tailTruncate = %q, want it to keep the tail", got)
	}
	if unchanged := tailTruncate("short", 20); unchanged != "short" {
		t.Errorf("tailTruncate shortened %q", unchanged)
	}
}

func hasSuffix(text, suffix string) bool {
	return strings.HasSuffix(text, suffix)
}
