package tui

import "charm.land/bubbles/v2/key"

// keyMap is every binding the explorer answers to. The movement keys follow both
// the arrow keys and the vi keys, since both are muscle memory for the people who
// live in a terminal.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDn   key.Binding
	Top      key.Binding
	Bottom   key.Binding
	Enter    key.Binding
	Back     key.Binding
	Root     key.Binding
	Tab      key.Binding
	Expand   key.Binding
	Collapse key.Binding

	// The detail pane has its own left/right and its own way out, so that the
	// same arrow keys mean "switch tab" there and "open / go back" in the list.
	PrevTab     key.Binding
	NextTab     key.Binding
	LeaveDetail key.Binding

	Filter  key.Binding
	Command key.Binding
	Refresh key.Binding
	Auto    key.Binding

	Watch   key.Binding
	Unwatch key.Binding
	Attrs   key.Binding
	Type    key.Binding
	Refs    key.Binding
	Copy    key.Binding

	Help   key.Binding
	Quit   key.Binding
	Cancel key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp: key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
		PageDn: key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),
		Top:    key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
		Bottom: key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
		// Enter changes which node the tree is rooted at; the arrow keys open and
		// close a branch in place, which is what makes the shape visible.
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "enter node")),
		Back:     key.NewBinding(key.WithKeys("esc", "backspace"), key.WithHelp("esc", "back")),
		Expand:   key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "expand")),
		Collapse: key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "collapse")),
		Root:     key.NewBinding(key.WithKeys("~"), key.WithHelp("~", "objects folder")),
		Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),

		PrevTab:     key.NewBinding(key.WithKeys("left", "h", "shift+tab"), key.WithHelp("←/h", "previous tab")),
		NextTab:     key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next tab")),
		LeaveDetail: key.NewBinding(key.WithKeys("esc", "backspace"), key.WithHelp("esc", "back to the list")),

		Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Command: key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "go to node")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Auto:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "auto refresh")),

		Watch:   key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch value")),
		Unwatch: key.NewBinding(key.WithKeys("W"), key.WithHelp("W", "clear watches")),
		Attrs:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "attributes")),
		Type:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "type")),
		Refs:    key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "references")),
		Copy:    key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy node id")),

		Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Cancel: key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "cancel")),
	}
}

// ShortHelp is the one-line hint strip in the footer.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Expand, k.Collapse, k.Enter, k.Back, k.Filter, k.Watch, k.Help, k.Quit}
}

// DetailHelp is the hint strip shown while the detail pane has the keyboard.
func (k keyMap) DetailHelp() []key.Binding {
	return []key.Binding{k.PrevTab, k.NextTab, k.Up, k.Down, k.LeaveDetail, k.Help, k.Quit}
}

// FullHelp is the grouped help shown by "?", arranged by what a group is for.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDn, k.Top, k.Bottom},
		{k.Expand, k.Collapse, k.Enter, k.Back, k.Root},
		{k.Command, k.Filter},
		{k.Tab, k.PrevTab, k.NextTab, k.LeaveDetail},
		{k.Attrs, k.Type, k.Refs, k.Copy},
		{k.Watch, k.Unwatch, k.Refresh, k.Auto},
		{k.Help, k.Quit},
	}
}
