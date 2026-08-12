package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// An open input bar showed nothing but a cursor before it was styled, which looks
// like a hung program. Every part a user needs - what the bar is, what it takes,
// and how to leave it - has to be on the line.
func TestOpenInputBarExplainsItself(t *testing.T) {
	for _, test := range []struct {
		key      string
		label    string
		mentions string
	}{
		{"/", "filter", "narrow"},
		{":", "go to", "node id"},
	} {
		t.Run(test.label, func(t *testing.T) {
			m := press(t, sized(t), test.key)

			footer := lastLines(m.View().Content, 1)
			if !strings.Contains(footer, test.label) {
				t.Errorf("the bar does not say what it is:\n%q", footer)
			}
			if !strings.Contains(footer, test.mentions) {
				t.Errorf("the bar does not say what it takes:\n%q", footer)
			}
			if !strings.Contains(footer, "esc") {
				t.Errorf("the bar does not say how to leave it:\n%q", footer)
			}

			// Once there is text, the placeholder gives way to it.
			typed := press(t, m, "x")
			footer = lastLines(typed.View().Content, 1)
			if !strings.Contains(footer, "x") {
				t.Errorf("the typed text is not shown:\n%q", footer)
			}
		})
	}
}

// The real cursor has to sit under the character being typed, or the terminal
// draws it at the top left and the bar looks inert.
func TestOpenInputPlacesTheCursor(t *testing.T) {
	m := press(t, sized(t), "/")

	view := m.View()
	if view.Cursor == nil {
		t.Fatal("no cursor is placed while the filter is open")
	}
	if view.Cursor.Position.Y != m.height-1 {
		t.Errorf("the cursor is on row %d, want the footer row %d", view.Cursor.Position.Y, m.height-1)
	}
	if view.Cursor.Position.X < m.inputMarkerWidth() {
		t.Errorf("the cursor is at column %d, want it past the %d-cell marker",
			view.Cursor.Position.X, m.inputMarkerWidth())
	}

	// The list has no cursor of its own to place.
	if closed := press(t, m, "esc"); closed.View().Cursor != nil {
		t.Error("a cursor is placed while no input is open")
	}
}

// The help overlay is the only documentation inside the explorer, so it has to
// carry the things the key list cannot say.
func TestHelpOverlayExplainsTheBars(t *testing.T) {
	m := press(t, sized(t), "?")
	content := ansi.Strip(m.View().Content)

	for _, want := range []string{
		"command bar", "filter", "Watching",
		"ns=2;s=Machine", "Server/ServerStatus", ":q",
		"press any key to go back",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("the help overlay is missing %q", want)
		}
	}

	for line := range strings.SplitSeq(content, "\n") {
		if width := lipgloss.Width(line); width > m.width {
			t.Errorf("a help line is %d cells wide, more than the %d-cell terminal: %q",
				width, m.width, line)
		}
	}
}

// With the detail pane focused the arrow keys belong to it: left and right move
// between its tabs rather than opening and closing nodes in the list.
func TestDetailPaneArrowsSwitchTabs(t *testing.T) {
	m := press(t, sized(t), "tab")
	if !m.focusDetail {
		t.Fatal("tab did not move the keyboard to the detail pane")
	}

	before := m.current.String()

	m = press(t, m, "right")
	if m.tab != tabType {
		t.Errorf("right moved to %v, want the type tab", m.tab)
	}
	m = press(t, m, "right")
	if m.tab != tabReferences {
		t.Errorf("right moved to %v, want the references tab", m.tab)
	}
	m = press(t, m, "left")
	if m.tab != tabType {
		t.Errorf("left moved to %v, want back to the type tab", m.tab)
	}

	// The tabs wrap rather than dead-ending at either edge.
	m = press(t, m, "left")
	m = press(t, m, "left")
	if m.tab != tabWatch {
		t.Errorf("left from the first tab moved to %v, want the last", m.tab)
	}
	m = press(t, m, "right")
	if m.tab != tabAttributes {
		t.Errorf("right from the last tab moved to %v, want the first", m.tab)
	}

	// None of that may have moved the list.
	if m.current.String() != before {
		t.Errorf("the arrow keys navigated the address space to %s", m.current)
	}
	if m.cursor != 0 {
		t.Errorf("the arrow keys moved the list cursor to %d", m.cursor)
	}
}

// Up and down scroll the detail pane, which is the only way to read content
// taller than the pane.
func TestDetailPaneArrowsScroll(t *testing.T) {
	m := press(t, sized(t), "tab")

	tall := make([]string, 200)
	for index := range tall {
		tall[index] = fmt.Sprintf("line %d", index)
	}
	m.detail.SetContent(strings.Join(tall, "\n"))

	if m.detail.YOffset() != 0 {
		t.Fatalf("the pane starts at offset %d, want the top", m.detail.YOffset())
	}

	m = press(t, m, "down")
	if m.detail.YOffset() == 0 {
		t.Error("down did not scroll the detail pane")
	}

	m = press(t, m, "G")
	if !m.detail.AtBottom() {
		t.Error("G did not reach the end of the detail pane")
	}
	m = press(t, m, "g")
	if !m.detail.AtTop() {
		t.Error("g did not return to the top of the detail pane")
	}

	// The scroll position is reported, since a pane with more below it looks the
	// same as one without.
	m.detail.GotoBottom()
	if content := ansi.Strip(m.View().Content); !strings.Contains(content, "end") {
		t.Error("the detail pane does not say that it is scrolled to the end")
	}
}

// Escape leaves the detail pane rather than walking back up the address space,
// which would be a surprising thing for a pane-local key to do.
func TestEscapeLeavesTheDetailPane(t *testing.T) {
	m := press(t, sized(t), "tab")
	before := m.current.String()

	m = press(t, m, "esc")
	if m.focusDetail {
		t.Error("escape did not return the keyboard to the list")
	}
	if m.current.String() != before {
		t.Errorf("escape moved the list to %s", m.current)
	}

	// Back in the list, escape goes up a level again.
	m = press(t, m, "enter")
	if m.current.String() == before {
		t.Fatal("enter did not open the selected node")
	}
	m = press(t, m, "esc")
	if m.current.String() != before {
		t.Errorf("escape in the list did not go back up, current is %s", m.current)
	}
}

// The footer has to show the keys that apply to whichever pane has the keyboard.
func TestFooterHintsFollowTheFocus(t *testing.T) {
	m := sized(t)

	list := lastLines(m.View().Content, 1)
	for _, want := range []string{"expand", "collapse", "enter node"} {
		if !strings.Contains(list, want) {
			t.Errorf("the list hints do not mention %q:\n%q", want, list)
		}
	}

	detail := lastLines(press(t, m, "tab").View().Content, 1)
	if !strings.Contains(detail, "tab") && !strings.Contains(detail, "previous") {
		t.Errorf("the detail hints do not mention the tabs:\n%q", detail)
	}
}

// lastLines returns the final n lines of a frame, with the styling stripped: a
// style change can fall between any two characters, so assertions about words
// have to be made against the plain text.
func lastLines(content string, n int) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return ansi.Strip(strings.Join(lines, "\n"))
}

// TestPrintFrames writes the rendered frames to stdout when SHOW_FRAMES is set,
// so the layout can be looked at rather than only asserted about:
//
//	SHOW_FRAMES=1 go test ./internal/tui -run TestPrintFrames -v
func TestPrintFrames(t *testing.T) {
	if os.Getenv("SHOW_FRAMES") == "" {
		t.Skip("set SHOW_FRAMES=1 to print the frames")
	}

	m := sized(t)
	for _, frame := range []struct {
		name  string
		model *model
	}{
		{"list", m},
		{"filter", press(t, m, "/")},
		{"command", press(t, m, ":")},
		{"help", press(t, m, "?")},
	} {
		t.Logf("\n=== %s ===\n%s\n", frame.name, frame.model.View().Content)
	}
}
