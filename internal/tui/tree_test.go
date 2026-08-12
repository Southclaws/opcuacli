package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// machineChildren is a plausible level under the machine object in level().
func machineChildren() []cligen.Node {
	return []cligen.Node{
		{NodeID: "ns=1;s=Machine/Sensors", BrowseName: "Sensors", NodeClass: "Object"},
		{NodeID: "ns=1;i=10", BrowseName: "Setpoint", NodeClass: "Variable"},
	}
}

// expandRow moves the cursor onto a named row and expands it, answering the
// browse the expansion triggers.
func expandRow(t *testing.T, m *model, name string, children []cligen.Node) *model {
	t.Helper()

	found := false
	for index, row := range m.rows {
		if row.node.BrowseName == name {
			m.cursor = index
			m.syncCursorID()
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no row named %q", name)
	}

	nodeID := m.rows[m.cursor].node.NodeID
	m = press(t, m, "right")

	if !m.loadingLevels[nodeID] {
		t.Fatalf("expanding %s did not ask for its level", name)
	}

	updated, _ := m.Update(levelMsg{parent: nodeID, nodes: children})
	return updated.(*model)
}

func TestExpandRevealsChildrenInPlace(t *testing.T) {
	m := sized(t)
	before := len(m.rows)
	root := m.current.String()

	m = expandRow(t, m, "Machine", machineChildren())

	if m.current.String() != root {
		t.Errorf("expanding moved the root to %s; only enter should do that", m.current)
	}
	if len(m.rows) != before+len(machineChildren()) {
		t.Fatalf("the tree has %d rows, want %d", len(m.rows), before+len(machineChildren()))
	}

	// The children sit directly under their parent, one level in.
	parent := m.rows[0]
	if parent.node.BrowseName != "Machine" || !parent.expanded {
		t.Fatalf("the first row is %q (expanded=%v), want an expanded Machine",
			parent.node.BrowseName, parent.expanded)
	}
	for _, name := range []string{"Sensors", "Setpoint"} {
		row, ok := rowNamed(m, name)
		if !ok {
			t.Fatalf("%s is not in the tree", name)
		}
		if row.depth != 1 {
			t.Errorf("%s is at depth %d, want 1", name, row.depth)
		}
		if row.guide == "" {
			t.Errorf("%s has no tree guide", name)
		}
	}
}

func TestCollapseHidesChildrenAgain(t *testing.T) {
	m := sized(t)
	before := len(m.rows)

	m = expandRow(t, m, "Machine", machineChildren())
	m.cursor = 0
	m.syncCursorID()

	m = press(t, m, "left")
	if len(m.rows) != before {
		t.Errorf("the tree has %d rows after collapsing, want %d", len(m.rows), before)
	}
	if m.expanded["ns=1;s=Machine"] {
		t.Error("the node is still marked expanded")
	}

	// The level stays cached, so expanding again costs nothing.
	m = press(t, m, "right")
	if m.loadingLevels["ns=1;s=Machine"] {
		t.Error("re-expanding fetched the level again; it should be cached")
	}
	if len(m.rows) != before+len(machineChildren()) {
		t.Errorf("re-expanding produced %d rows, want %d", len(m.rows), before+len(machineChildren()))
	}
}

// Expanding a node that is already open steps into it, so holding the key walks
// down a chain.
func TestExpandOnAnOpenNodeStepsIn(t *testing.T) {
	m := sized(t)
	m = expandRow(t, m, "Machine", machineChildren())

	m.cursor = 0
	m.syncCursorID()
	m = press(t, m, "right")

	row, ok := m.row()
	if !ok {
		t.Fatal("nothing is selected")
	}
	if row.node.BrowseName != "Sensors" {
		t.Errorf("the cursor moved to %q, want the first child", row.node.BrowseName)
	}
}

// Collapsing a closed node walks back out to its parent, which is how a tree is
// left without reaching for another key.
func TestCollapseOnAClosedChildMovesToTheParent(t *testing.T) {
	m := sized(t)
	m = expandRow(t, m, "Machine", machineChildren())

	child, ok := rowIndex(m, "Setpoint")
	if !ok {
		t.Fatal("Setpoint is not in the tree")
	}
	m.cursor = child
	m.syncCursorID()

	m = press(t, m, "left")
	row, _ := m.row()
	if row.node.BrowseName != "Machine" {
		t.Errorf("the cursor moved to %q, want the parent", row.node.BrowseName)
	}
}

// A node found to have nothing under it stops offering to expand, so the marker
// tells the truth once the level is known.
func TestAnEmptyLevelBecomesALeaf(t *testing.T) {
	m := sized(t)
	m = expandRow(t, m, "Machine", nil)

	row, ok := rowNamed(m, "Machine")
	if !ok {
		t.Fatal("Machine is not in the tree")
	}
	if row.expanded {
		t.Error("a node with no children is marked expanded")
	}
	if row.expandable {
		t.Error("a node known to be empty still offers to expand")
	}
}

// Entering a node still re-roots the tree, which is what keeps a deep address
// space navigable once a branch is too far in to read.
func TestEnterStillRerootsAndResetsTheTree(t *testing.T) {
	m := sized(t)
	m = expandRow(t, m, "Machine", machineChildren())

	m.cursor = 0
	m.syncCursorID()
	m = press(t, m, "enter")

	if m.current.String() != "ns=1;s=Machine" {
		t.Fatalf("enter moved to %s, want the selected node", m.current)
	}
	if len(m.rows) != 0 {
		t.Errorf("the new root starts with %d rows, want none until its level arrives", len(m.rows))
	}
	if len(m.expanded) != 0 {
		t.Errorf("the expansion state carried over into the new root: %v", m.expanded)
	}
	if len(m.stack) != 1 {
		t.Errorf("the breadcrumb has %d entries, want 1", len(m.stack))
	}
}

// The cursor follows the node it is on, not its row number: expanding something
// above the selection must not move the selection.
func TestTheCursorStaysOnItsNodeWhenTheTreeGrows(t *testing.T) {
	m := sized(t)

	target, ok := rowIndex(m, "Restart")
	if !ok {
		t.Fatal("Restart is not in the tree")
	}
	m.cursor = target
	m.syncCursorID()

	m = expandRow(t, m, "Machine", machineChildren())

	// expandRow moved the cursor to Machine to expand it; move back and grow the
	// tree above the selection.
	back, _ := rowIndex(m, "Restart")
	m.cursor = back
	m.syncCursorID()

	m.levels["ns=1;s=Machine/Sensors"] = []cligen.Node{
		{NodeID: "ns=1;i=20", BrowseName: "Sensor01", NodeClass: "Variable"},
	}
	m.expanded["ns=1;s=Machine/Sensors"] = true
	m.rebuild()

	row, _ := m.row()
	if row.node.BrowseName != "Restart" {
		t.Errorf("the cursor is on %q after the tree grew, want Restart", row.node.BrowseName)
	}
}

// An address space is a graph: the Server object is reachable from itself. A tree
// that followed that would never finish drawing.
func TestExpandingACycleTerminates(t *testing.T) {
	m := sized(t)

	m.levels["i=85"] = []cligen.Node{
		{NodeID: "ns=1;s=A", BrowseName: "A", NodeClass: "Object"},
	}
	m.levels["ns=1;s=A"] = []cligen.Node{
		{NodeID: "ns=1;s=B", BrowseName: "B", NodeClass: "Object"},
	}
	m.levels["ns=1;s=B"] = []cligen.Node{
		{NodeID: "ns=1;s=A", BrowseName: "A", NodeClass: "Object"},
	}
	m.expanded["ns=1;s=A"] = true
	m.expanded["ns=1;s=B"] = true

	m.rebuild()

	if len(m.rows) == 0 {
		t.Fatal("a cyclic tree drew nothing")
	}
	if len(m.rows) > 8 {
		t.Errorf("a cyclic tree drew %d rows; the cycle was followed", len(m.rows))
	}
}

// A filter keeps a branch whose descendant matches, since hiding the branch would
// hide the match with it.
func TestFilterKeepsTheAncestorsOfAMatch(t *testing.T) {
	m := sized(t)
	m = expandRow(t, m, "Machine", machineChildren())

	m.filter = "setpoint"
	m.rebuild()

	names := map[string]bool{}
	for _, row := range m.rows {
		names[row.node.BrowseName] = true
	}
	if !names["Setpoint"] {
		t.Error("the matching node was filtered out")
	}
	if !names["Machine"] {
		t.Error("the parent of the match was filtered out, hiding the match")
	}
	if names["Sensors"] {
		t.Error("a sibling that does not match was kept")
	}
}

// The tree is what the pane is for, so its shape has to be visible: guides for
// the nesting and a marker for what can be opened.
func TestTreeIsDrawnWithGuidesAndMarkers(t *testing.T) {
	m := sized(t)
	m = expandRow(t, m, "Machine", machineChildren())

	content := ansi.Strip(m.View().Content)

	if !strings.Contains(content, "▾") {
		t.Error("an expanded node is not marked")
	}
	if !strings.Contains(content, "▸") {
		t.Error("a node that can be expanded is not marked")
	}
	if !strings.Contains(content, "└─") || !strings.Contains(content, "├─") {
		t.Errorf("the nesting is not drawn:\n%s", content)
	}
	if !strings.Contains(content, "shown") {
		t.Error("the header does not say how much of the tree is open")
	}
}

func rowNamed(m *model, name string) (treeRow, bool) {
	for _, row := range m.rows {
		if row.node.BrowseName == name {
			return row, true
		}
	}
	return treeRow{}, false
}

func rowIndex(m *model, name string) (int, bool) {
	for index, row := range m.rows {
		if row.node.BrowseName == name {
			return index, true
		}
	}
	return 0, false
}
