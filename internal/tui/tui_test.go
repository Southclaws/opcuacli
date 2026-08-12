package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/conn"
	"github.com/Southclaws/opcuacli/internal/opc"
	"github.com/Southclaws/opcuacli/internal/render"
)

// newTestModel builds a model with no client, which is enough to drive the
// update loop and the layout: every call to a server arrives as a message, and
// the tests send those messages directly.
func newTestModel() *model {
	start, _ := opc.ParseNodeID("i=85")
	theme := render.ThemeByName("charm", true)

	return &model{
		ctx:          context.Background(),
		client:       &conn.Client{Endpoint: "opc.tcp://plant:4840", Policy: "None", Mode: "None", TokenType: "Anonymous"},
		theme:        theme,
		refresh:      2 * time.Second,
		current:      start,
		keys:         defaultKeys(),
		help:         newHelp(theme),
		spinner:      spinner.New(),
		detail:       viewport.New(),
		filterInput:  newFilterInput(theme),
		commandInput: newCommandInput(theme),
		watchHandles: map[uint32]string{},
		values:       map[string]cligen.ReadResult{},
		watched:      map[string]*watchRow{},
		tab:          tabAttributes,

		levels:        map[string][]cligen.Node{},
		expanded:      map[string]bool{},
		loadingLevels: map[string]bool{},
	}
}

// level is a plausible answer to a browse of the Objects folder.
func level() []cligen.Node {
	return []cligen.Node{
		{NodeID: "ns=1;s=Machine", BrowseName: "Machine", NodeClass: "Object"},
		{NodeID: "ns=1;i=1", BrowseName: "COOK_KTL_01_TEMP_C", NodeClass: "Variable"},
		{NodeID: "ns=1;i=2", BrowseName: "COOK_KTL_01_FIRING", NodeClass: "Variable"},
		{NodeID: "ns=1;s=Restart", BrowseName: "Restart", NodeClass: "Method"},
	}
}

// sized returns a model that has been told the terminal size and given a level,
// which is the state every interaction test starts from.
func sized(t *testing.T) *model {
	t.Helper()

	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(*model)

	updated, _ = m.Update(levelMsg{parent: "i=85", nodes: level()})
	return updated.(*model)
}

func press(t *testing.T, m *model, key string) *model {
	t.Helper()

	updated, _ := m.Update(tea.KeyPressMsg{Code: keyCode(key), Text: key})
	return updated.(*model)
}

// keyCode maps the keys these tests press to their rune, which is how a key press
// identifies itself.
func keyCode(key string) rune {
	if len(key) == 1 {
		return rune(key[0])
	}
	switch key {
	case "enter":
		return tea.KeyEnter
	case "esc":
		return tea.KeyEscape
	case "tab":
		return tea.KeyTab
	case "left":
		return tea.KeyLeft
	case "right":
		return tea.KeyRight
	case "up":
		return tea.KeyUp
	case "down":
		return tea.KeyDown
	}
	return 0
}

func TestViewRendersTheWholeScreen(t *testing.T) {
	m := sized(t)

	view := m.View()
	if !view.AltScreen {
		t.Error("the explorer should take the alternate screen")
	}

	content := view.Content
	for _, want := range []string{"opcua", "opc.tcp://plant:4840", "Machine", "COOK_KTL_01_TEMP_C", "Restart"} {
		if !strings.Contains(content, want) {
			t.Errorf("the view is missing %q:\n%s", want, content)
		}
	}

	// The layout must fit the terminal it was told about, or the terminal wraps
	// it and the panes shear apart.
	lines := strings.Split(content, "\n")
	if len(lines) > 30 {
		t.Errorf("the view is %d lines tall, want at most 30", len(lines))
	}
}

func TestViewBeforeTheFirstResizeDoesNotPanic(t *testing.T) {
	m := newTestModel()

	if content := m.View().Content; content == "" {
		t.Error("the view is empty before the terminal size is known")
	}
}

func TestCursorMovesAndStaysInRange(t *testing.T) {
	m := sized(t)

	if m.cursor != 0 {
		t.Fatalf("the cursor starts at %d, want 0", m.cursor)
	}

	m = press(t, m, "j")
	if m.cursor != 1 {
		t.Errorf("after j the cursor is at %d, want 1", m.cursor)
	}

	m = press(t, m, "G")
	if m.cursor != len(level())-1 {
		t.Errorf("after G the cursor is at %d, want the last row", m.cursor)
	}

	// Moving past the end must clamp rather than run off the slice.
	for range 5 {
		m = press(t, m, "j")
	}
	if m.cursor != len(level())-1 {
		t.Errorf("the cursor ran to %d, want it clamped to the last row", m.cursor)
	}

	m = press(t, m, "g")
	if m.cursor != 0 {
		t.Errorf("after g the cursor is at %d, want 0", m.cursor)
	}
	for range 3 {
		m = press(t, m, "k")
	}
	if m.cursor != 0 {
		t.Errorf("the cursor ran to %d, want it clamped to 0", m.cursor)
	}
}

func TestSelectedFollowsTheCursor(t *testing.T) {
	m := sized(t)
	m = press(t, m, "j")

	selected, ok := m.selected()
	if !ok {
		t.Fatal("nothing is selected")
	}
	if selected.BrowseName != "COOK_KTL_01_TEMP_C" {
		t.Errorf("selected %q, want the second row", selected.BrowseName)
	}
}

func TestFilterNarrowsTheLevel(t *testing.T) {
	m := sized(t)

	m = press(t, m, "/")
	if m.mode != modeFilter {
		t.Fatalf("mode is %v after /, want the filter", m.mode)
	}

	for _, key := range strings.Split("FIRING", "") {
		m = press(t, m, key)
	}
	if len(m.rows) != 1 {
		t.Fatalf("%d row(s) match FIRING, want 1", len(m.rows))
	}

	// A filter that matches nothing says so rather than looking like an empty
	// address space.
	m = press(t, m, "esc")
	if m.mode != modeList || m.filter != "" {
		t.Errorf("escape left mode %v and filter %q, want the list and no filter", m.mode, m.filter)
	}
	if len(m.rows) != len(level()) {
		t.Errorf("%d row(s) visible after clearing the filter, want all", len(m.rows))
	}
}

// While an input is open it owns the keyboard: a node id containing "q" must not
// quit, and "/" must not start a nested filter.
func TestAnOpenInputOwnsTheKeyboard(t *testing.T) {
	m := sized(t)
	m = press(t, m, ":")

	if m.mode != modeCommand {
		t.Fatalf("mode is %v after :, want the command bar", m.mode)
	}

	for _, key := range strings.Split("q/w", "") {
		m = press(t, m, key)
	}
	if m.mode != modeCommand {
		t.Fatalf("mode is %v while typing, want the command bar", m.mode)
	}
	if got := m.commandInput.Value(); got != "q/w" {
		t.Errorf("the command bar holds %q, want q/w", got)
	}
}

func TestDescendAndAscend(t *testing.T) {
	m := sized(t)

	m = press(t, m, "enter")
	if got := m.current.String(); got != "ns=1;s=Machine" {
		t.Fatalf("opened %s, want ns=1;s=Machine", got)
	}
	if len(m.stack) != 1 {
		t.Fatalf("the breadcrumb has %d entry(s), want 1", len(m.stack))
	}

	// The cursor position is remembered per level, so going back returns to the
	// node you came from.
	m = press(t, m, "esc")
	if got := m.current.String(); got != "i=85" {
		t.Fatalf("went back to %s, want i=85", got)
	}
	if len(m.stack) != 0 {
		t.Errorf("the breadcrumb has %d entry(s) at the root, want none", len(m.stack))
	}

	// Going back from the top is a no-op rather than an error.
	m = press(t, m, "esc")
	if got := m.current.String(); got != "i=85" {
		t.Errorf("going back from the root moved to %s", got)
	}
}

func TestDescendRemembersTheCursor(t *testing.T) {
	m := sized(t)
	m = press(t, m, "j")
	m = press(t, m, "j")
	m = press(t, m, "j") // the Restart method
	m = press(t, m, "enter")

	updated, _ := m.Update(levelMsg{parent: "ns=1;s=Restart", nodes: nil})
	m = updated.(*model)

	m = press(t, m, "esc")
	updated, _ = m.Update(levelMsg{parent: "i=85", nodes: level()})
	m = updated.(*model)

	if got := m.cursorID; got != "ns=1;s=Restart" {
		t.Errorf("the cursor came back on %q, want the node it left from", got)
	}
	if m.cursor != 3 {
		t.Errorf("the cursor came back at row %d, want the row that node is on", m.cursor)
	}
}

func TestTabsSwitchTheDetailPane(t *testing.T) {
	m := sized(t)

	for key, want := range map[string]tab{"t": tabType, "f": tabReferences, "a": tabAttributes} {
		m = press(t, m, key)
		if m.tab != want {
			t.Errorf("after %q the detail pane shows %v, want %v", key, m.tab, want)
		}
	}
}

func TestDetailPaneShowsAttributes(t *testing.T) {
	m := sized(t)
	m = press(t, m, "j")

	updated, _ := m.Update(attributesMsg{
		nodeID: "ns=1;i=1",
		set: cligen.AttributeSet{
			NodeID:    "ns=1;i=1",
			NodeClass: "Variable",
			Attributes: []cligen.Attribute{
				{ID: 13, Name: "Value", Status: "Good", Text: new("97.2")},
				{ID: 14, Name: "DataType", Status: "Good", Text: new("Double")},
			},
		},
	})
	m = updated.(*model)

	content := m.View().Content
	for _, want := range []string{"Value", "97.2", "DataType", "Double"} {
		if !strings.Contains(content, want) {
			t.Errorf("the detail pane is missing %q:\n%s", want, content)
		}
	}
}

// A late answer for a level the user has already left must not replace what is on
// screen.
func TestALateLevelAnswerIsIgnored(t *testing.T) {
	m := sized(t)

	updated, _ := m.Update(levelMsg{parent: "ns=1;s=SomewhereElse", nodes: []cligen.Node{
		{NodeID: "ns=1;i=99", BrowseName: "Stale", NodeClass: "Variable"},
	}})
	m = updated.(*model)

	if len(m.rows) != len(level()) {
		t.Errorf("the level was replaced by a stale answer: %d row(s)", len(m.rows))
	}
}

func TestBrowseFailureIsReported(t *testing.T) {
	m := sized(t)

	updated, _ := m.Update(levelMsg{parent: "i=85", err: context.DeadlineExceeded})
	m = updated.(*model)

	if !strings.Contains(m.View().Content, context.DeadlineExceeded.Error()) {
		t.Errorf("the failure is not on screen:\n%s", m.View().Content)
	}
}

func TestValuesAppearInTheList(t *testing.T) {
	m := sized(t)

	updated, _ := m.Update(valuesMsg{
		parent: "i=85",
		values: map[string]cligen.ReadResult{
			"ns=1;i=1": {NodeID: "ns=1;i=1", Status: "Good", Text: new("97.2")},
		},
	})
	m = updated.(*model)

	if !strings.Contains(m.View().Content, "97.2") {
		t.Errorf("the value is not in the list:\n%s", m.View().Content)
	}
}

func TestWatchRequiresAVariable(t *testing.T) {
	m := sized(t)

	// The cursor starts on an object, which cannot be watched.
	m = press(t, m, "w")
	if len(m.watchOrder) != 0 {
		t.Error("an object was added to the watch panel")
	}
	if m.status == "" {
		t.Error("refusing to watch an object said nothing")
	}
}

func TestWatchPanelRendersLiveValues(t *testing.T) {
	m := sized(t)

	// The panel is filled directly: creating a real subscription needs a server.
	m.watched["ns=1;i=1"] = &watchRow{nodeID: "ns=1;i=1", name: "COOK_KTL_01_TEMP_C", status: "…"}
	m.watchOrder = []string{"ns=1;i=1"}
	m.watchHandles[1] = "ns=1;i=1"
	m.tab = tabWatch

	stamp := time.Now()
	for _, value := range []float64{96, 97, 98} {
		updated, _ := m.Update(watchMsg{
			nodeID: "ns=1;i=1",
			value: &ua.DataValue{
				Status:          ua.StatusOK,
				Value:           ua.MustVariant(value),
				SourceTimestamp: stamp,
			},
		})
		m = updated.(*model)
	}

	row := m.watched["ns=1;i=1"]
	if row.updates != 3 {
		t.Errorf("the row counted %d update(s), want 3", row.updates)
	}
	if len(row.history) != 3 {
		t.Errorf("the row kept %d point(s) of history, want 3", len(row.history))
	}

	content := m.View().Content
	if !strings.Contains(content, "COOK_KTL_01_TEMP_C") || !strings.Contains(content, "98") {
		t.Errorf("the watch panel is missing its row:\n%s", content)
	}
}

// A long-running watch must not accumulate history without bound.
func TestWatchHistoryIsBounded(t *testing.T) {
	m := sized(t)
	m.watched["ns=1;i=1"] = &watchRow{nodeID: "ns=1;i=1", name: "T"}
	m.watchOrder = []string{"ns=1;i=1"}

	for i := range 200 {
		m.applyWatch(watchMsg{
			nodeID: "ns=1;i=1",
			value:  &ua.DataValue{Status: ua.StatusOK, Value: ua.MustVariant(float64(i))},
		})
	}

	if got := len(m.watched["ns=1;i=1"].history); got > 60 {
		t.Errorf("history grew to %d points, want a bounded window", got)
	}
}

func TestClearWatches(t *testing.T) {
	m := sized(t)
	m.watched["ns=1;i=1"] = &watchRow{nodeID: "ns=1;i=1", name: "T"}
	m.watchOrder = []string{"ns=1;i=1"}

	m = press(t, m, "W")
	if len(m.watchOrder) != 0 || len(m.watched) != 0 {
		t.Error("clearing the watches left rows behind")
	}
}

func TestHelpOverlayCoversTheScreenAndReturns(t *testing.T) {
	m := sized(t)

	m = press(t, m, "?")
	if m.mode != modeHelp {
		t.Fatalf("mode is %v after ?, want help", m.mode)
	}

	content := m.View().Content
	if !strings.Contains(content, "explorer") {
		t.Errorf("the help overlay does not look like help:\n%s", content)
	}

	m = press(t, m, "x")
	if m.mode != modeList {
		t.Errorf("mode is %v after a key press in help, want the list", m.mode)
	}
}

func TestAutoRefreshToggles(t *testing.T) {
	m := sized(t)

	m = press(t, m, "R")
	if !m.autoRefresh {
		t.Error("R did not switch auto refresh on")
	}
	m = press(t, m, "R")
	if m.autoRefresh {
		t.Error("R did not switch auto refresh off again")
	}
}

func TestQuitReturnsTheQuitCommand(t *testing.T) {
	m := sized(t)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q produced no command, want quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("q produced %T, want a quit message", cmd())
	}
}

func TestGoToParsesANodeID(t *testing.T) {
	m := sized(t)

	if cmd := m.goTo("i=2253"); cmd == nil {
		t.Fatal("jumping to a node id produced no command")
	}
	if got := m.current.String(); got != "i=2253" {
		t.Errorf("jumped to %s, want i=2253", got)
	}

	// A bad node id is reported rather than followed.
	m.status = ""
	if cmd := m.goTo("ns=nonsense"); cmd != nil {
		t.Error("jumping to a bad node id produced a command")
	}
	if m.status == "" {
		t.Error("a bad node id said nothing")
	}
}

func TestGoToQuits(t *testing.T) {
	m := sized(t)

	cmd := m.goTo("q")
	if cmd == nil {
		t.Fatal(":q produced no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf(":q produced %T, want a quit message", cmd())
	}
}

func TestLayoutSharesTheWidthBetweenThePanes(t *testing.T) {
	m := sized(t)

	if m.listWidth()+m.detailWidth() > 120 {
		t.Errorf("the panes are %d + %d wide, more than the terminal",
			m.listWidth(), m.detailWidth())
	}
	if m.listHeight() < 3 {
		t.Errorf("the list is %d lines tall, want at least a few", m.listHeight())
	}

	// A tiny terminal must still produce a usable layout rather than negative
	// widths.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 6})
	small := updated.(*model)
	if small.listWidth() < 1 || small.detailWidth() < 1 || small.listHeight() < 1 {
		t.Errorf("a small terminal produced %d x %d panes and %d lines",
			small.listWidth(), small.detailWidth(), small.listHeight())
	}
	if content := small.View().Content; content == "" {
		t.Error("a small terminal rendered nothing")
	}
}
