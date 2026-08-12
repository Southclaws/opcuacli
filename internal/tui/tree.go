package tui

import (
	"strings"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// The left pane is a tree rather than one level at a time, because the shape of
// an address space is most of what a person is trying to learn from it. A level
// is fetched once and cached; expanding a node reveals the cached level under it,
// so the topology builds up as you walk it and nothing is re-fetched to go back.
//
// State lives in three maps keyed by node id rather than in a tree of pointers:
// reloading a level then means replacing one map entry, and expansion survives a
// refresh, a jump, and going back up.

// treeRow is one rendered line of the tree.
type treeRow struct {
	node  cligen.Node
	depth int
	// guide is the box-drawing prefix that shows how this row hangs off its
	// ancestors.
	guide string
	// expandable is false only once a node has been loaded and found to have no
	// children; until then every node is assumed to have some, because a server
	// does not say in advance.
	expandable bool
	expanded   bool
	loading    bool
	// parent is the row index of this row's parent, or -1 at the top level.
	parent int
}

// children returns a cached level, if it has been loaded.
func (m *model) children(parentID string) ([]cligen.Node, bool) {
	nodes, ok := m.levels[parentID]
	return nodes, ok
}

// rebuild recomputes the visible rows from the cached levels and the expansion
// state, keeping the cursor on the same node where it can.
func (m *model) rebuild() {
	previous := m.cursorID

	m.rows = m.rows[:0]
	m.walk(m.current.String(), 0, "", map[string]bool{m.current.String(): true})

	// The cursor follows the node it was on rather than its position: expanding a
	// node above it must not move the selection out from under the user.
	m.cursor = 0
	for index, row := range m.rows {
		if row.node.NodeID == previous {
			m.cursor = index
			break
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(len(m.rows)-1, 0)
	}
	m.syncCursorID()
}

// walk appends the rows for one level and, for each expanded node, the rows of
// its own level.
//
// ancestors guards against the cycles a reference graph contains: the Server
// object is reachable from itself, and a tree that followed that would never
// finish drawing.
func (m *model) walk(parentID string, depth int, prefix string, ancestors map[string]bool) {
	nodes, loaded := m.children(parentID)
	if !loaded {
		return
	}

	visible := make([]cligen.Node, 0, len(nodes))
	for _, node := range nodes {
		if m.keep(node, ancestors) {
			visible = append(visible, node)
		}
	}

	parentIndex := len(m.rows) - 1
	for index, node := range visible {
		last := index == len(visible)-1

		branch, continuation := "├─ ", "│  "
		if last {
			branch, continuation = "└─ ", "   "
		}

		guide := ""
		if depth > 0 {
			guide = prefix + branch
		}

		childLevel, childLoaded := m.children(node.NodeID)
		row := treeRow{
			node:       node,
			depth:      depth,
			guide:      guide,
			expandable: !childLoaded || len(childLevel) > 0,
			expanded:   m.expanded[node.NodeID] && childLoaded && len(childLevel) > 0,
			loading:    m.loadingLevels[node.NodeID],
			parent:     parentIndex,
		}
		if depth == 0 {
			row.parent = -1
		}
		m.rows = append(m.rows, row)

		if !row.expanded || ancestors[node.NodeID] {
			continue
		}

		childPrefix := prefix
		if depth > 0 {
			childPrefix = prefix + continuation
		}

		ancestors[node.NodeID] = true
		m.walk(node.NodeID, depth+1, childPrefix, ancestors)
		delete(ancestors, node.NodeID)
	}
}

// keep reports whether a node survives the filter.
//
// A filtered tree keeps a node whose own name does not match but which has a
// visible descendant that does, since hiding the branch would hide the match
// with it.
func (m *model) keep(node cligen.Node, ancestors map[string]bool) bool {
	if m.filter == "" {
		return true
	}
	if matches(node, strings.ToLower(m.filter)) {
		return true
	}
	if !m.expanded[node.NodeID] || ancestors[node.NodeID] {
		return false
	}

	children, loaded := m.children(node.NodeID)
	if !loaded {
		return false
	}

	ancestors[node.NodeID] = true
	defer delete(ancestors, node.NodeID)

	for _, child := range children {
		if m.keep(child, ancestors) {
			return true
		}
	}
	return false
}

// syncCursorID records which node the cursor is on, so that the selection can be
// restored after the rows are rebuilt.
func (m *model) syncCursorID() {
	if row, ok := m.row(); ok {
		m.cursorID = row.node.NodeID
		return
	}
	m.cursorID = ""
}

// row is the row under the cursor.
func (m *model) row() (treeRow, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return treeRow{}, false
	}
	return m.rows[m.cursor], true
}

// selected is the node under the cursor.
func (m *model) selected() (cligen.Node, bool) {
	row, ok := m.row()
	return row.node, ok
}

// rootCount is how many nodes the current level holds, before filtering.
func (m *model) rootCount() int {
	return len(m.levels[m.current.String()])
}

// reset clears the tree, for a move to a different root node.
func (m *model) reset() {
	m.levels = map[string][]cligen.Node{}
	m.expanded = map[string]bool{}
	m.loadingLevels = map[string]bool{}
	m.values = map[string]cligen.ReadResult{}
	m.rows = m.rows[:0]
	m.cursor = 0
	m.cursorID = ""
	m.filter = ""
}

// visibleVariables are the node ids of every variable currently on screen, which
// is the set worth re-reading on a refresh.
func (m *model) visibleVariables() []cligen.Node {
	nodes := make([]cligen.Node, 0, len(m.rows))
	for _, row := range m.rows {
		if row.node.NodeClass == "Variable" {
			nodes = append(nodes, row.node)
		}
	}
	return nodes
}
