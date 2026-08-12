// Package tui implements the full-screen address space explorer.
//
// The layout follows the pattern a terminal operator already knows from tools
// like k9s: a header saying what you are connected to and where you are, one
// resource list you move through, a detail pane for the selected item, and a
// footer of keys. Everything on screen comes from the same operations the
// commands use, so what the explorer shows and what a script would read cannot
// drift apart.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/conn"
	"github.com/Southclaws/opcuacli/internal/opc"
	"github.com/Southclaws/opcuacli/internal/render"
)

// Options configure the explorer.
type Options struct {
	// Node is the node to open at.
	Node string
	// Refresh is how often visible values are re-read and how fast the watch
	// subscription publishes.
	Refresh time.Duration
	// Theme names the palette.
	Theme string
	// Watch are nodes to add to the watch panel on startup.
	Watch []string
}

// Run opens the explorer on an already-connected client and returns when the
// user quits.
func Run(ctx context.Context, client *conn.Client, options Options) error {
	start, err := opc.ParseNodeID(options.Node)
	if err != nil {
		return err
	}
	if options.Refresh <= 0 {
		options.Refresh = 2 * time.Second
	}

	theme := render.ThemeByName(options.Theme, true)

	m := &model{
		ctx:          ctx,
		client:       client,
		theme:        theme,
		refresh:      options.Refresh,
		current:      start,
		keys:         defaultKeys(),
		help:         newHelp(theme),
		spinner:      spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Styles.Accent)),
		detail:       viewport.New(),
		filterInput:  newFilterInput(theme),
		commandInput: newCommandInput(theme),
		watchHandles: map[uint32]string{},
		values:       map[string]cligen.ReadResult{},
		watched:      map[string]*watchRow{},
		pendingWatch: options.Watch,
		loading:      true,
		tab:          tabAttributes,

		levels:        map[string][]cligen.Node{},
		expanded:      map[string]bool{},
		loadingLevels: map[string]bool{},
	}

	program := tea.NewProgram(m, tea.WithContext(ctx))
	_, err = program.Run()

	m.stopWatch()
	return err
}

// newFilterInput builds the bar that narrows the current level.
func newFilterInput(theme render.Theme) textinput.Model {
	return newInput(theme, "filter", "type to narrow this level, enter keeps it, esc clears it")
}

// newCommandInput builds the bar that jumps to a node.
func newCommandInput(theme render.Theme) textinput.Model {
	return newInput(theme, "go to", "node id, name, or browse path - :q quits")
}

// newInput builds one of the two input bars.
//
// The styles are set from the theme rather than left at their defaults: an
// unstyled prompt on a dark background is invisible, which makes an open input
// look like a frozen program. The label and placeholder say what the bar is for,
// since a bar with only a cursor in it explains nothing.
func newInput(theme render.Theme, label, placeholder string) textinput.Model {
	input := textinput.New()
	input.Prompt = label + " "
	input.Placeholder = placeholder
	input.CharLimit = 200

	styles := textinput.DefaultStyles(theme.Dark)
	styles.Focused.Prompt = theme.Styles.Accent.Bold(true)
	styles.Focused.Text = theme.Styles.Value
	styles.Focused.Placeholder = theme.Styles.Faint
	styles.Blurred.Prompt = theme.Styles.Dim
	styles.Blurred.Text = theme.Styles.Dim
	styles.Cursor.Color = theme.Colors.Accent
	styles.Cursor.Blink = true
	input.SetStyles(styles)

	// The terminal's own cursor is used rather than a drawn block, so it blinks
	// and sits where the terminal expects it; the view places it at the input's
	// position on screen.
	input.SetVirtualCursor(false)

	return input
}

// newHelp builds the key-hint renderer with readable contrast: the defaults are
// two shades of grey, which on a dark background leaves the descriptions barely
// legible.
func newHelp(theme render.Theme) help.Model {
	model := help.New()

	styles := help.DefaultStyles(theme.Dark)
	styles.ShortKey = theme.Styles.Accent
	styles.ShortDesc = theme.Styles.Dim
	styles.ShortSeparator = theme.Styles.Faint
	styles.FullKey = theme.Styles.Accent.Bold(true)
	styles.FullDesc = theme.Styles.Value
	styles.FullSeparator = theme.Styles.Faint
	styles.Ellipsis = theme.Styles.Faint
	model.Styles = styles

	return model
}

// mode is what the keyboard is currently doing.
type mode int

const (
	modeList mode = iota
	modeFilter
	modeCommand
	modeHelp
)

// tab is which view the detail pane is showing.
type tab int

const (
	tabAttributes tab = iota
	tabType
	tabReferences
	tabWatch
)

func (t tab) String() string {
	switch t {
	case tabType:
		return "type"
	case tabReferences:
		return "references"
	case tabWatch:
		return "watch"
	default:
		return "attributes"
	}
}

// crumb is one level of the path walked into the address space, remembering which
// node the cursor was on so that going back returns to where you came from.
type crumb struct {
	node   *ua.NodeID
	name   string
	cursor string
}

// watchRow is one live value in the watch panel.
type watchRow struct {
	nodeID  string
	name    string
	value   string
	status  string
	stamp   time.Time
	updates int
	history []float64
}

type model struct {
	ctx    context.Context
	client *conn.Client
	theme  render.Theme

	width  int
	height int

	// Address space position. The tree of the current root is held as cached
	// levels plus an expansion set, both keyed by node id; see tree.go.
	current       *ua.NodeID
	stack         []crumb
	levels        map[string][]cligen.Node
	expanded      map[string]bool
	loadingLevels map[string]bool
	rows          []treeRow
	cursor        int
	// cursorID is the node the cursor is on, so the selection survives the rows
	// being rebuilt under it.
	cursorID string
	values   map[string]cligen.ReadResult

	// Detail pane.
	tab        tab
	detail     viewport.Model
	detailNode string

	// Watch panel.
	subscription  *opcua.Subscription
	notifications chan *opcua.PublishNotificationData
	watchHandles  map[uint32]string
	watched       map[string]*watchRow
	watchOrder    []string
	pendingWatch  []string

	// Interaction.
	mode         mode
	keys         keyMap
	help         help.Model
	spinner      spinner.Model
	filterInput  textinput.Model
	commandInput textinput.Model
	filter       string
	status       string
	failure      string
	loading      bool
	autoRefresh  bool
	refresh      time.Duration
	focusDetail  bool
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.browseLevel(m.current), m.tick())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case levelMsg:
		delete(m.loadingLevels, msg.parent)
		if msg.parent == m.current.String() {
			m.loading = false
		}
		if msg.err != nil {
			// A level that fails to load is reported without discarding the tree
			// around it: one unreadable branch does not invalidate the rest.
			if msg.parent == m.current.String() {
				m.failure = msg.err.Error()
			} else {
				m.status = msg.err.Error()
			}
			m.rebuild()
			return m, nil
		}

		if msg.parent == m.current.String() {
			m.failure = ""
		}
		m.levels[msg.parent] = msg.nodes
		if len(msg.nodes) == 0 {
			// Nothing under it: leave it collapsed so the row shows as a leaf.
			delete(m.expanded, msg.parent)
		}
		m.rebuild()
		return m, tea.Batch(m.readValues(msg.parent, msg.nodes), m.loadDetail(), m.watchPending())

	case jumpMsg:
		m.loading = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.stack = append(m.stack, crumb{node: m.current, name: m.currentName(), cursor: m.cursorID})
		m.current = msg.node
		m.reset()
		m.levels[msg.node.String()] = msg.nodes
		m.rebuild()
		return m, tea.Batch(m.readValues(msg.node.String(), msg.nodes), m.loadDetail())

	case valuesMsg:
		// Values are keyed by node id and a tree shows several levels at once, so
		// an answer applies wherever its node is on screen.
		for nodeID, result := range msg.values {
			m.values[nodeID] = result
		}
		return m, nil

	case attributesMsg:
		if msg.err != nil {
			m.detail.SetContent(m.theme.Styles.Bad.Render(msg.err.Error()))
			return m, nil
		}
		if msg.nodeID == m.detailNode && m.tab == tabAttributes {
			m.detail.SetContent(m.renderAttributes(msg.set))
		}
		return m, nil

	case typeMsg:
		if msg.err != nil {
			m.detail.SetContent(m.theme.Styles.Bad.Render(msg.err.Error()))
			return m, nil
		}
		if msg.nodeID == m.detailNode && m.tab == tabType {
			m.detail.SetContent(m.renderType(msg.info))
		}
		return m, nil

	case referencesMsg:
		if msg.err != nil {
			m.detail.SetContent(m.theme.Styles.Bad.Render(msg.err.Error()))
			return m, nil
		}
		if msg.nodeID == m.detailNode && m.tab == tabReferences {
			m.detail.SetContent(m.renderReferences(msg.references))
		}
		return m, nil

	case watchMsg:
		m.applyWatch(msg)
		if m.tab == tabWatch {
			m.detail.SetContent(m.renderWatch())
		}
		return m, m.listenWatch()

	case watchErrMsg:
		m.status = "watch stopped: " + msg.err.Error()
		return m, nil

	case statusMsg:
		if msg != "" {
			m.status = string(msg)
		}
		return m, nil

	case tickMsg:
		cmds := []tea.Cmd{m.tick()}
		if m.autoRefresh {
			cmds = append(cmds, m.readValues(m.current.String(), m.visibleVariables()))
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

// handleKey routes a key press by mode: the text inputs own the keyboard while
// they are open, so a node id containing "q" cannot quit the program.
func (m *model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeFilter:
		switch {
		case key.Matches(msg, m.keys.Cancel):
			m.mode = modeList
			m.filterInput.Blur()
			m.filter = ""
			m.rebuild()
			return m, nil
		case msg.String() == "enter":
			m.mode = modeList
			m.filterInput.Blur()
			return m, nil
		}

		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.filter = m.filterInput.Value()
		m.rebuild()
		return m, cmd

	case modeCommand:
		switch {
		case key.Matches(msg, m.keys.Cancel):
			m.mode = modeList
			m.commandInput.Blur()
			return m, nil
		case msg.String() == "enter":
			target := strings.TrimSpace(m.commandInput.Value())
			m.mode = modeList
			m.commandInput.Blur()
			m.commandInput.SetValue("")
			return m, m.goTo(target)
		}

		var cmd tea.Cmd
		m.commandInput, cmd = m.commandInput.Update(msg)
		return m, cmd

	case modeHelp:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
		m.mode = modeList
		return m, nil
	}

	// Quitting and help work wherever the keyboard is.
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.mode = modeHelp
		return m, nil

	case key.Matches(msg, m.keys.Tab):
		m.focusDetail = !m.focusDetail
		return m, nil
	}

	// With the detail pane focused, the movement keys belong to it: left and
	// right change tab and up and down scroll, which is what the arrow keys mean
	// once the cursor is no longer in the list.
	if m.focusDetail {
		if model, cmd, handled := m.handleDetailKey(msg); handled {
			return model, cmd
		}
	}

	switch {

	case key.Matches(msg, m.keys.Filter):
		m.mode = modeFilter
		m.filterInput.SetValue("")
		m.filter = ""
		return m, m.filterInput.Focus()

	case key.Matches(msg, m.keys.Command):
		m.mode = modeCommand
		return m, m.commandInput.Focus()

	case key.Matches(msg, m.keys.Refresh):
		m.loading = true
		return m, tea.Batch(m.browseLevel(m.current), m.loadDetail())

	case key.Matches(msg, m.keys.Auto):
		m.autoRefresh = !m.autoRefresh
		m.status = fmt.Sprintf("auto refresh %s", onOff(m.autoRefresh))
		return m, nil

	case key.Matches(msg, m.keys.Root):
		m.stack = nil
		objects, err := opc.ParseNodeID("Objects")
		if err != nil {
			return m, nil
		}
		m.current = objects
		m.cursor = 0
		m.loading = true
		return m, m.browseLevel(m.current)

	case key.Matches(msg, m.keys.Expand):
		return m, m.expand()

	case key.Matches(msg, m.keys.Collapse):
		return m, m.collapse()

	case key.Matches(msg, m.keys.Enter):
		return m, m.descend()

	case key.Matches(msg, m.keys.Back):
		return m, m.ascend()

	case key.Matches(msg, m.keys.Attrs):
		m.tab = tabAttributes
		return m, m.loadDetail()

	case key.Matches(msg, m.keys.Type):
		m.tab = tabType
		return m, m.loadDetail()

	case key.Matches(msg, m.keys.Refs):
		m.tab = tabReferences
		return m, m.loadDetail()

	case key.Matches(msg, m.keys.Watch):
		return m, m.watchSelected()

	case key.Matches(msg, m.keys.Unwatch):
		m.clearWatches()
		return m, nil

	case key.Matches(msg, m.keys.Copy):
		selected, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.status = "copied " + selected.NodeID
		return m, tea.SetClipboard(selected.NodeID)
	}

	if m.focusDetail {
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}

	return m, m.moveCursor(msg)
}

// handleDetailKey handles the keys that mean something different while the detail
// pane has the keyboard, reporting whether it took the key. Anything it does not
// take falls through to the shared bindings, so `a`, `w` and the rest still work
// from either pane.
func (m *model) handleDetailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.NextTab):
		m.tab = nextTab(m.tab, 1)
		return m, m.loadDetail(), true

	case key.Matches(msg, m.keys.PrevTab):
		m.tab = nextTab(m.tab, -1)
		return m, m.loadDetail(), true

	case key.Matches(msg, m.keys.LeaveDetail):
		m.focusDetail = false
		return m, nil, true

	case key.Matches(msg, m.keys.Top):
		m.detail.GotoTop()
		return m, nil, true

	case key.Matches(msg, m.keys.Bottom):
		m.detail.GotoBottom()
		return m, nil, true

	case key.Matches(msg, m.keys.Up), key.Matches(msg, m.keys.Down),
		key.Matches(msg, m.keys.PageUp), key.Matches(msg, m.keys.PageDn):
		// The viewport's own key map covers these, and it knows how far its
		// content extends.
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd, true
	}

	return m, nil, false
}

// nextTab steps through the detail tabs, wrapping around so the arrow keys never
// dead-end.
func nextTab(current tab, step int) tab {
	tabs := []tab{tabAttributes, tabType, tabReferences, tabWatch}

	for index, candidate := range tabs {
		if candidate != current {
			continue
		}
		next := (index + step + len(tabs)) % len(tabs)
		return tabs[next]
	}
	return tabAttributes
}

// moveCursor handles list movement and reloads the detail pane for the node the
// cursor lands on.
func (m *model) moveCursor(msg tea.KeyPressMsg) tea.Cmd {
	before := m.cursor
	page := max(m.listHeight()-1, 1)

	switch {
	case key.Matches(msg, m.keys.Up):
		m.cursor--
	case key.Matches(msg, m.keys.Down):
		m.cursor++
	case key.Matches(msg, m.keys.PageUp):
		m.cursor -= page
	case key.Matches(msg, m.keys.PageDn):
		m.cursor += page
	case key.Matches(msg, m.keys.Top):
		m.cursor = 0
	case key.Matches(msg, m.keys.Bottom):
		m.cursor = len(m.rows) - 1
	default:
		return nil
	}

	m.cursor = clamp(m.cursor, 0, max(len(m.rows)-1, 0))
	m.syncCursorID()
	if m.cursor == before {
		return nil
	}
	return m.loadDetail()
}

// expand opens the branch under the cursor, fetching its level the first time.
//
// A node already open steps into its first child instead, so holding the arrow
// key walks down a chain the way it does in a file tree.
func (m *model) expand() tea.Cmd {
	row, ok := m.row()
	if !ok {
		return nil
	}

	if row.expanded {
		if m.cursor+1 < len(m.rows) && m.rows[m.cursor+1].depth > row.depth {
			m.cursor++
			m.syncCursorID()
			return m.loadDetail()
		}
		return nil
	}
	if !row.expandable {
		return nil
	}

	m.expanded[row.node.NodeID] = true

	if _, loaded := m.children(row.node.NodeID); loaded {
		m.rebuild()
		return nil
	}

	target, err := opc.ParseNodeID(row.node.NodeID)
	if err != nil {
		m.status = err.Error()
		return nil
	}

	m.loadingLevels[row.node.NodeID] = true
	m.rebuild()
	return m.browseLevel(target)
}

// collapse closes the branch under the cursor, or moves to its parent when it is
// already closed - which is how a tree is walked back out without reaching for
// another key.
func (m *model) collapse() tea.Cmd {
	row, ok := m.row()
	if !ok {
		return nil
	}

	if row.expanded {
		delete(m.expanded, row.node.NodeID)
		m.rebuild()
		return nil
	}

	if row.parent >= 0 && row.parent < len(m.rows) {
		m.cursor = row.parent
		m.syncCursorID()
		return m.loadDetail()
	}
	return nil
}

// descend opens the selected node, remembering where the cursor was.
func (m *model) descend() tea.Cmd {
	selected, ok := m.selected()
	if !ok {
		return nil
	}
	target, err := opc.ParseNodeID(selected.NodeID)
	if err != nil {
		m.failure = err.Error()
		return nil
	}

	m.stack = append(m.stack, crumb{node: m.current, name: m.currentName(), cursor: m.cursorID})
	m.current = target
	m.reset()
	m.loading = true
	return m.browseLevel(target)
}

// ascend goes back up one level.
func (m *model) ascend() tea.Cmd {
	if len(m.stack) == 0 {
		return nil
	}

	previous := m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	m.current = previous.node
	m.reset()
	// The cursor is restored by node id once the level arrives, which is why it is
	// set before the browse rather than after.
	m.cursorID = previous.cursor
	m.loading = true
	return m.browseLevel(previous.node)
}

// goTo jumps to a node id or a browse path, which is what the command bar is for:
// a deep node is faster to name than to walk to.
func (m *model) goTo(target string) tea.Cmd {
	if target == "" {
		return nil
	}
	if target == "q" || target == "quit" {
		return tea.Quit
	}

	if strings.Contains(target, "/") {
		client := m.client
		ctx := m.ctx
		root := m.current

		return func() tea.Msg {
			resolved, err := opc.Resolve(ctx, client, root, []string{target}, 0)
			if err != nil {
				return statusMsg("resolve: " + err.Error())
			}
			if len(resolved) == 0 || resolved[0].NodeID == nil {
				return statusMsg(fmt.Sprintf("%s did not resolve", target))
			}
			node, err := opc.ParseNodeID(*resolved[0].NodeID)
			if err != nil {
				return statusMsg(err.Error())
			}
			nodes, err := opc.Browse(ctx, client, opc.BrowseOptions{
				Root: node, Depth: 1, Direction: ua.BrowseDirectionForward, IncludeSubtypes: true,
			})
			return jumpMsg{node: node, nodes: nodes, err: err}
		}
	}

	node, err := opc.ParseNodeID(target)
	if err != nil {
		m.status = err.Error()
		return nil
	}

	m.stack = append(m.stack, crumb{node: m.current, name: m.currentName(), cursor: m.cursorID})
	m.current = node
	m.reset()
	m.loading = true
	return m.browseLevel(node)
}

// jumpMsg is the answer to a resolved browse path: the node it named and its
// children, so the jump lands in one update.
type jumpMsg struct {
	node  *ua.NodeID
	nodes []cligen.Node
	err   error
}

// watchSelected adds the selected variable to the watch panel.
func (m *model) watchSelected() tea.Cmd {
	selected, ok := m.selected()
	if !ok {
		return nil
	}
	if selected.NodeClass != "Variable" {
		m.status = "only a variable can be watched"
		return nil
	}
	if _, exists := m.watched[selected.NodeID]; exists {
		m.tab = tabWatch
		m.detail.SetContent(m.renderWatch())
		return nil
	}

	target, err := opc.ParseNodeID(selected.NodeID)
	if err != nil {
		m.status = err.Error()
		return nil
	}

	first := m.subscription == nil
	if err := m.addWatch(target); err != nil {
		m.status = "watch: " + err.Error()
		return nil
	}

	m.watched[selected.NodeID] = &watchRow{
		nodeID: selected.NodeID,
		name:   selected.BrowseName,
		status: "…",
	}
	m.watchOrder = append(m.watchOrder, selected.NodeID)
	m.tab = tabWatch
	m.detail.SetContent(m.renderWatch())
	m.status = fmt.Sprintf("watching %d node(s)", len(m.watchOrder))

	if first {
		return m.listenWatch()
	}
	return nil
}

// watchPending adds the nodes named by --watch, once, after the first level has
// loaded. Waiting until then means the failure of one bad node id is reported in
// the footer rather than before the screen exists.
func (m *model) watchPending() tea.Cmd {
	if len(m.pendingWatch) == 0 {
		return nil
	}

	pending := m.pendingWatch
	m.pendingWatch = nil

	first := m.subscription == nil
	added := 0
	for _, text := range pending {
		target, err := opc.ParseNodeID(text)
		if err != nil {
			m.status = err.Error()
			continue
		}
		if _, exists := m.watched[target.String()]; exists {
			continue
		}
		if err := m.addWatch(target); err != nil {
			m.status = "watch " + text + ": " + err.Error()
			continue
		}
		m.watched[target.String()] = &watchRow{
			nodeID: target.String(),
			name:   watchName(target),
			status: "…",
		}
		m.watchOrder = append(m.watchOrder, target.String())
		added++
	}

	if added == 0 {
		return nil
	}
	m.tab = tabWatch
	m.detail.SetContent(m.renderWatch())
	if first {
		return m.listenWatch()
	}
	return nil
}

// watchName labels a watched node: its standard name, or the identifier part of
// its node id, which is what distinguishes it from its siblings.
func watchName(target *ua.NodeID) string {
	if name := opc.StandardName(target); name != "" {
		return name
	}
	text := target.String()
	if index := strings.LastIndex(text, ";"); index >= 0 {
		return text[index+1:]
	}
	return text
}

// applyWatch records a live value against its row.
func (m *model) applyWatch(msg watchMsg) {
	row, ok := m.watched[msg.nodeID]
	if !ok || msg.value == nil {
		return
	}

	row.value = opc.Text(msg.value.Value)
	row.status = opc.StatusName(msg.value.Status)
	row.updates++
	row.stamp = msg.value.SourceTimestamp
	if row.stamp.IsZero() {
		row.stamp = time.Now()
	}
	if number, ok := opc.Numeric(msg.value.Value); ok {
		row.history = append(row.history, number)
		if len(row.history) > 60 {
			row.history = row.history[len(row.history)-60:]
		}
	}
}

// clearWatches cancels the subscription and empties the panel.
func (m *model) clearWatches() {
	m.stopWatch()
	m.watched = map[string]*watchRow{}
	m.watchOrder = nil
	m.watchHandles = map[uint32]string{}
	m.status = "watches cleared"
	if m.tab == tabWatch {
		m.detail.SetContent(m.renderWatch())
	}
}

// stopWatch cancels the watch subscription, if there is one.
func (m *model) stopWatch() {
	if m.subscription == nil {
		return
	}
	// The program's context is usually already cancelled when this runs during
	// shutdown, so cancelling needs a context of its own.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(m.ctx), 2*time.Second)
	defer cancel()
	_ = m.subscription.Cancel(ctx)
	m.subscription = nil
}

// loadDetail refreshes the detail pane for the selected node.
func (m *model) loadDetail() tea.Cmd {
	if m.tab == tabWatch {
		m.detail.SetContent(m.renderWatch())
		return nil
	}

	selected, ok := m.selected()
	if !ok {
		m.detailNode = ""
		m.detail.SetContent(m.theme.Styles.Faint.Render("nothing selected"))
		return nil
	}
	m.detailNode = selected.NodeID

	switch m.tab {
	case tabType:
		m.detail.SetContent(m.theme.Styles.Faint.Render("reading type…"))
		return m.loadType(selected.NodeID)
	case tabReferences:
		m.detail.SetContent(m.theme.Styles.Faint.Render("reading references…"))
		return m.loadReferences(selected.NodeID)
	default:
		m.detail.SetContent(m.theme.Styles.Faint.Render("reading attributes…"))
		return m.loadAttributes(selected.NodeID)
	}
}

func matches(child cligen.Node, needle string) bool {
	if strings.Contains(strings.ToLower(child.BrowseName), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(child.NodeID), needle) {
		return true
	}
	if child.DisplayName != nil && strings.Contains(strings.ToLower(*child.DisplayName), needle) {
		return true
	}
	return false
}

func (m *model) currentName() string {
	if name := opc.StandardName(m.current); name != "" {
		return name
	}
	return m.current.String()
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

// layout resizes the panes to the terminal.
func (m *model) layout() {
	m.help.SetWidth(m.width)
	m.detail.SetWidth(m.detailWidth())
	// The detail pane spends one line on its tab strip, so its viewport is a
	// line shorter than the list beside it; giving it the full height would make
	// the body taller than the terminal and shear the panes apart.
	m.detail.SetHeight(max(m.listHeight()-1, 1))

	// An input pads its text area out to the width it is given, and draws its
	// prompt in front of that, so the room left for the text is the line minus
	// the marker, the hint and the prompt itself. Without subtracting all three
	// the input fills the line and pushes the hint off it.
	for _, input := range []*textinput.Model{&m.filterInput, &m.commandInput} {
		room := m.width - m.inputMarkerWidth() - inputHintWidth - lipgloss.Width(input.Prompt)
		input.SetWidth(max(room, 12))
	}
}

// inputHintWidth is the room reserved for the keys shown at the end of an open
// input bar.
const inputHintWidth = 14

// The header is a title line and a breadcrumb line; the footer is one line of
// keys plus one of status.
const (
	headerHeight = 3
	footerHeight = 2
)

func (m *model) listHeight() int {
	return max(m.height-headerHeight-footerHeight, 3)
}

func (m *model) listWidth() int {
	// The list gets the larger share: names and values are the reason the
	// explorer exists, and the detail pane is a key/value list that reads fine
	// narrow.
	return max(m.width*3/5, 24)
}

func (m *model) detailWidth() int {
	return max(m.width-m.listWidth()-3, 20)
}

func (m *model) View() tea.View {
	view := tea.NewView("")
	view.AltScreen = true
	view.WindowTitle = "opcua · " + m.client.Endpoint

	if m.width == 0 || m.height == 0 {
		view.SetContent("starting…")
		return view
	}

	if m.mode == modeHelp {
		view.SetContent(m.renderHelp())
		return view
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderList(),
		m.divider(),
		m.renderDetail(),
	)

	view.SetContent(lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(),
		body,
		m.renderFooter(),
	))
	if input, open := m.openInput(); open {
		// The input knows where its cursor sits within itself; the offset is
		// where the input is drawn on screen, so the terminal puts the real
		// cursor under the character being typed.
		cursor := input.Cursor()
		if cursor != nil {
			cursor.Position.X += m.inputMarkerWidth()
			cursor.Position.Y += m.height - 1
			view.Cursor = cursor
		}
	}
	return view
}

// openInput is whichever input bar has the keyboard, if either does.
func (m *model) openInput() (textinput.Model, bool) {
	switch m.mode {
	case modeFilter:
		return m.filterInput, true
	case modeCommand:
		return m.commandInput, true
	default:
		return textinput.Model{}, false
	}
}
