package render

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// newTestPrinter writes to a buffer, which is not a terminal, so styling is
// stripped and the assertions can be about content and layout.
func newTestPrinter(width int) (*Printer, *bytes.Buffer) {
	buffer := &bytes.Buffer{}
	return NewPrinter(buffer, Options{NoColor: true, Width: width}), buffer
}

func TestTableSizesToItsContent(t *testing.T) {
	printer, buffer := newTestPrinter(100)

	printer.Table([]string{"A", "B"}, [][]string{{"one", "two"}})

	for line := range strings.SplitSeq(strings.TrimSpace(buffer.String()), "\n") {
		if width := lipgloss.Width(line); width > 20 {
			t.Errorf("line %q is %d cells wide; a small table should not pad to the terminal", line, width)
		}
	}
}

// A table wider than the terminal is constrained to it, so the output cannot
// wrap in the terminal itself and break the row structure.
func TestTableIsCappedAtTheTerminalWidth(t *testing.T) {
	printer, buffer := newTestPrinter(40)

	printer.Table(
		[]string{"NODE", "VALUE"},
		[][]string{{strings.Repeat("x", 60), strings.Repeat("y", 60)}},
	)

	for line := range strings.SplitSeq(strings.TrimSpace(buffer.String()), "\n") {
		if width := lipgloss.Width(line); width > 40 {
			t.Errorf("line is %d cells wide, want at most 40", width)
		}
	}
}

func TestTableSaysWhenThereIsNothing(t *testing.T) {
	printer, buffer := newTestPrinter(80)

	printer.Table([]string{"A"}, nil)

	if !strings.Contains(buffer.String(), "no results") {
		t.Errorf("an empty table rendered %q, want it to say so", buffer.String())
	}
}

// Plain output is what pipes into other tools, so it must carry no styling and no
// header: tab-separated fields only.
func TestPlainIsTabSeparated(t *testing.T) {
	printer, buffer := newTestPrinter(80)

	printer.Plain([][]string{{"ns=1;i=1", "21.5"}, {"ns=1;i=2", "3"}})

	want := "ns=1;i=1\t21.5\nns=1;i=2\t3\n"
	if buffer.String() != want {
		t.Errorf("Plain wrote %q, want %q", buffer.String(), want)
	}
}

func TestKVAlignsItsKeys(t *testing.T) {
	printer, buffer := newTestPrinter(80)

	printer.KV([]Field{F("Short", "a"), F("A much longer key", "b")})

	// Only the trailing newline is trimmed: the leading padding of the first
	// line is exactly what alignment means here.
	lines := strings.Split(strings.TrimRight(buffer.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("KV wrote %d line(s), want 2", len(lines))
	}
	if strings.Index(lines[0], "a") != strings.Index(lines[1], "b") {
		t.Errorf("values are not aligned:\n%q\n%q", lines[0], lines[1])
	}
}

func TestKVBlankFieldIsABlankLine(t *testing.T) {
	printer, buffer := newTestPrinter(80)

	printer.KV([]Field{F("A", "1"), {}, F("B", "2")})

	if lines := strings.Split(buffer.String(), "\n"); len(lines) != 4 {
		t.Errorf("KV wrote %d line(s), want 3 plus a trailing newline: %q", len(lines), buffer.String())
	}
}

func TestTreeShowsTheHierarchy(t *testing.T) {
	printer, buffer := newTestPrinter(80)

	printer.Tree("root", []TreeNode{
		{Label: "one", Children: []TreeNode{{Label: "deep"}}},
		{Label: "two"},
	})

	output := buffer.String()
	for _, want := range []string{"root", "one", "deep", "two"} {
		if !strings.Contains(output, want) {
			t.Errorf("tree output is missing %q:\n%s", want, output)
		}
	}
	if strings.Index(output, "deep") < strings.Index(output, "one") {
		t.Error("a child was rendered before its parent")
	}
}

func TestSparkline(t *testing.T) {
	printer, _ := newTestPrinter(80)

	if got := printer.Sparkline(nil); got != "" {
		t.Errorf("Sparkline(nil) = %q, want an empty string", got)
	}

	rising := printer.Sparkline([]float64{1, 2, 3, 4})
	if lipgloss.Width(rising) != 4 {
		t.Fatalf("Sparkline drew %q, want one glyph per point", rising)
	}

	glyphs := []rune(rising)
	if glyphs[0] == glyphs[3] {
		t.Errorf("a rising series drew %q, want the last glyph taller than the first", rising)
	}

	// A flat series has no shape; every glyph should be the same one.
	flat := []rune(printer.Sparkline([]float64{2, 2, 2}))
	if flat[0] != flat[1] || flat[1] != flat[2] {
		t.Errorf("a flat series drew %q, want one repeated glyph", string(flat))
	}
}

func TestTruncate(t *testing.T) {
	for _, test := range []struct {
		input string
		width int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly-10", 10, "exactly-10"},
		{"truncate me please", 8, "truncat…"},
		{"anything", 1, "…"},
		{"anything", 0, "anything"},
	} {
		if got := Truncate(test.input, test.width); got != test.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", test.input, test.width, got, test.want)
		}
	}

	// Whatever the width, the result must fit inside it.
	for width := 1; width < 12; width++ {
		if got := lipgloss.Width(Truncate("a wide-enough string", width)); got > width {
			t.Errorf("Truncate to %d produced %d cells", width, got)
		}
	}
}

// A live view redraws in place on a terminal. Writing to a buffer is not a
// terminal, so each frame is appended instead - which keeps a redirected stream
// readable as a log rather than filling it with cursor movements.
func TestLiveAppendsWhenNotATerminal(t *testing.T) {
	printer, buffer := newTestPrinter(80)

	live := printer.Live()
	live.Render("frame one")
	live.Render("frame two")
	live.Finish()

	output := buffer.String()
	if !strings.Contains(output, "frame one") || !strings.Contains(output, "frame two") {
		t.Errorf("live output = %q, want both frames", output)
	}
	if strings.Contains(output, "\x1b[") {
		t.Errorf("live output contains cursor movements on a non-terminal: %q", output)
	}
}

func TestThemesAreAllUsable(t *testing.T) {
	for _, name := range ThemeNames {
		for _, dark := range []bool{true, false} {
			theme := ThemeByName(name, dark)
			if theme.Name != name {
				t.Errorf("ThemeByName(%q) is named %q", name, theme.Name)
			}
			if theme.Colors.Text == nil || theme.Colors.Accent == nil || theme.Colors.Bad == nil {
				t.Errorf("theme %q leaves a colour unset", name)
			}
			if len(theme.Symbols.Sparks) == 0 {
				t.Errorf("theme %q has no sparkline glyphs", name)
			}
			if theme.Styles.Title.Render("x") == "" {
				t.Errorf("theme %q renders nothing", name)
			}
		}
	}

	if got := ThemeByName("no-such-theme", true).Name; got != "charm" {
		t.Errorf("an unknown theme resolved to %q, want charm", got)
	}
}
