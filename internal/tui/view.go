package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/render"
)

// renderHeader draws the title bar and the breadcrumb: what server this is, and
// where in its address space the cursor sits.
func (m *model) renderHeader() string {
	styles := m.theme.Styles

	badge := styles.Badge.Render(" opcua ")
	endpoint := styles.Subtitle.Render(m.client.Endpoint)
	security := styles.Faint.Render(m.client.Security())

	indicators := []string{}
	if m.autoRefresh {
		indicators = append(indicators, styles.Good.Render("auto "+m.refresh.String()))
	}
	if len(m.watchOrder) > 0 {
		indicators = append(indicators, styles.Accent.Render(fmt.Sprintf("%s %d watched",
			m.theme.Symbols.Live, len(m.watchOrder))))
	}
	if m.loading {
		indicators = append(indicators, m.spinner.View()+styles.Faint.Render(" loading"))
	}

	left := strings.Join([]string{badge, endpoint, security}, " ")
	right := strings.Join(indicators, styles.Faint.Render(" · "))
	title := m.spread(left, right)

	crumbs := make([]string, 0, len(m.stack)+1)
	for _, entry := range m.stack {
		crumbs = append(crumbs, styles.Dim.Render(entry.name))
	}
	crumbs = append(crumbs, styles.Title.Render(m.currentName()))
	path := strings.Join(crumbs, styles.Faint.Render(" › "))

	// The count says how big this level is and how much of the tree below it is
	// open, which is also what tells you a filter is hiding something.
	summary := fmt.Sprintf("%d nodes", m.rootCount())
	switch {
	case m.filter != "":
		summary = fmt.Sprintf("%d shown of %d, matching %q", len(m.rows), m.rootCount(), m.filter)
	case len(m.rows) > m.rootCount():
		summary = fmt.Sprintf("%d nodes · %d shown", m.rootCount(), len(m.rows))
	}
	breadcrumb := m.spread(path, styles.Faint.Render(summary))

	rule := styles.Border.Render(strings.Repeat("─", m.width))
	return strings.Join([]string{title, breadcrumb, rule}, "\n")
}

// spread puts left and right on one line, pushed to the edges of the terminal.
func (m *model) spread(left, right string) string {
	return m.spreadWithin(left, right, m.width)
}

// spreadWithin is spread inside a pane rather than the whole terminal.
func (m *model) spreadWithin(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return render.Truncate(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// scrollHint says how far down a scrollable pane the view has reached, which is
// the only sign that there is more below.
func scrollHint(percent float64) string {
	switch {
	case percent <= 0:
		return "top"
	case percent >= 1:
		return "end"
	default:
		return fmt.Sprintf("%d%%", int(percent*100))
	}
}

// renderList draws the node list, one row per child, with the value of a variable
// shown inline so a level can be read at a glance.
func (m *model) renderList() string {
	styles := m.theme.Styles
	width := m.listWidth()
	height := m.listHeight()

	if len(m.rows) == 0 {
		message := "this node has no children"
		if m.filter != "" {
			message = fmt.Sprintf("nothing matches %q", m.filter)
		}
		if m.loading {
			message = "loading…"
		}
		if m.failure != "" {
			message = m.failure
		}
		return lipgloss.NewStyle().Width(width).Height(height).Render(styles.Faint.Render("  " + message))
	}

	// Scroll the window so the cursor stays inside it.
	start := 0
	if m.cursor >= height {
		start = m.cursor - height + 1
	}
	end := min(start+height, len(m.rows))

	lines := make([]string, 0, height)
	for position := start; position < end; position++ {
		lines = append(lines, m.renderRow(m.rows[position], position == m.cursor, width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines, "\n")
}

// renderRow is one line of the tree: the guides that show where the node hangs,
// a marker saying whether it can be opened, its class, its name, and its value.
func (m *model) renderRow(row treeRow, selected bool, width int) string {
	styles := m.theme.Styles
	child := row.node

	marker := "  "
	if selected {
		marker = styles.Accent.Render("▌ ")
	}

	guide := styles.Border.Render(row.guide)

	// The chevron is the affordance for expanding: a node whose level has not
	// been read yet is assumed to have one, because a server does not say in
	// advance which nodes are leaves.
	chevron := "  "
	switch {
	case row.loading:
		chevron = styles.Accent.Render("· ")
	case row.expanded:
		chevron = styles.Accent.Render("▾ ")
	case row.expandable:
		chevron = styles.Faint.Render("▸ ")
	}

	class := classBadge(m.theme, child.NodeClass)
	name := child.BrowseName
	if child.DisplayName != nil && *child.DisplayName != "" {
		name = *child.DisplayName
	}

	value := ""
	if result, ok := m.values[child.NodeID]; ok {
		text := derefOr(result.Text, "")
		if strings.HasPrefix(result.Status, "Bad") {
			value = styles.Bad.Render(render.Truncate(result.Status, 20))
		} else {
			value = styles.Accent.Render(render.Truncate(text, 22))
		}
	}

	// Lay the row out by measuring, so a wide glyph in a name cannot push the
	// value column out of alignment.
	fixed := lipgloss.Width(marker) + lipgloss.Width(row.guide) + lipgloss.Width(chevron) + 6
	nameWidth := max(width-fixed-lipgloss.Width(value)-2, 8)
	nameCell := pad(render.Truncate(name, nameWidth), nameWidth)
	if selected {
		nameCell = styles.Value.Bold(true).Render(nameCell)
	} else {
		nameCell = styles.Value.Render(nameCell)
	}

	line := marker + guide + chevron + class + " " + nameCell + " " + value
	return pad(render.Truncate(line, width), width)
}

// classBadge is the fixed-width class marker at the start of a row.
func classBadge(theme render.Theme, class string) string {
	short := map[string]string{
		"Object":        "obj ",
		"Variable":      "var ",
		"Method":        "fn  ",
		"ObjectType":    "otyp",
		"VariableType":  "vtyp",
		"ReferenceType": "rtyp",
		"DataType":      "dtyp",
		"View":          "view",
	}[class]
	if short == "" {
		short = "    "
	}

	switch class {
	case "Variable":
		return theme.Styles.Subtitle.Render(short)
	case "Method":
		return theme.Styles.Class.Render(short)
	case "Object":
		return theme.Styles.Dim.Render(short)
	default:
		return theme.Styles.Type.Render(short)
	}
}

// divider draws the vertical rule between the panes.
func (m *model) divider() string {
	line := m.theme.Styles.Border.Render("│")
	lines := make([]string, m.listHeight())
	for index := range lines {
		lines[index] = " " + line + " "
	}
	return strings.Join(lines, "\n")
}

// renderDetail draws the detail pane with its tab strip.
func (m *model) renderDetail() string {
	styles := m.theme.Styles
	width := m.detailWidth()

	tabs := make([]string, 0, 4)
	for _, candidate := range []tab{tabAttributes, tabType, tabReferences, tabWatch} {
		label := candidate.String()
		if candidate == tabWatch && len(m.watchOrder) > 0 {
			label = fmt.Sprintf("%s (%d)", label, len(m.watchOrder))
		}

		switch {
		case candidate == m.tab && m.focusDetail:
			// The selected tab is marked while the pane has the keyboard, so it
			// is clear which tab the arrow keys will move away from.
			tabs = append(tabs, styles.Selected.Bold(true).Render(" "+label+" "))
		case candidate == m.tab:
			tabs = append(tabs, styles.Accent.Bold(true).Render(label))
		default:
			tabs = append(tabs, styles.Faint.Render(label))
		}
	}

	strip := strings.Join(tabs, styles.Faint.Render(" · "))

	// The scroll position is the more useful of the two annotations - it is the
	// only sign that there is content below - so it claims the space first, and
	// the arrow hint is added only if the line still has room for it.
	scroll := ""
	if m.detail.TotalLineCount() > m.detail.VisibleLineCount() {
		scroll = styles.Faint.Render(scrollHint(m.detail.ScrollPercent()))
	}

	const arrowHint = "  ← →"
	if m.focusDetail && lipgloss.Width(strip)+lipgloss.Width(scroll)+lipgloss.Width(arrowHint) <= width {
		strip += styles.Faint.Render(arrowHint)
	}

	header := m.spreadWithin(strip, scroll, width)

	return lipgloss.JoinVertical(lipgloss.Left,
		pad(header, width),
		m.detail.View(),
	)
}

// renderFooter draws the key hints, or whichever input is open, plus the status
// line.
func (m *model) renderFooter() string {
	styles := m.theme.Styles
	rule := styles.Border.Render(strings.Repeat("─", m.width))

	status := m.status
	if m.failure != "" {
		status = styles.Bad.Render(m.failure)
	} else if status != "" {
		status = styles.Dim.Render(status)
	}

	switch m.mode {
	case modeFilter:
		return strings.Join([]string{rule, m.renderInput(m.filterInput)}, "\n")
	case modeCommand:
		return strings.Join([]string{rule, m.renderInput(m.commandInput)}, "\n")
	}

	// The hints follow the keyboard: with the detail pane focused, the arrow keys
	// and escape do something else, and showing the list's keys would be wrong.
	bindings := m.keys.ShortHelp()
	if m.focusDetail {
		bindings = m.keys.DetailHelp()
	}

	// The status matters more than the key hints, and neither can be truncated
	// safely because both carry styling. When they will not both fit, the status
	// takes the line; the keys are always a "?" away.
	hints := m.help.ShortHelpView(bindings)
	line := m.spread(hints, status)
	if status != "" && lipgloss.Width(hints)+lipgloss.Width(status)+2 > m.width {
		line = status
	}
	return strings.Join([]string{rule, line}, "\n")
}

// renderInput draws an open input bar: a marker so the line cannot be mistaken
// for empty space, the input itself, and the keys that close it.
func (m *model) renderInput(input textinput.Model) string {
	styles := m.theme.Styles

	marker := styles.Badge.Render(" " + m.theme.Symbols.Arrow + " ")
	hint := styles.Faint.Render("enter · esc")

	line := marker + " " + input.View()
	gap := m.width - lipgloss.Width(line) - lipgloss.Width(hint) - 1
	if gap < 1 {
		return line
	}
	return line + strings.Repeat(" ", gap) + hint
}

// inputMarkerWidth is how far the input's own text starts from the left edge of
// the footer, which the cursor position has to account for.
func (m *model) inputMarkerWidth() int {
	return lipgloss.Width(m.theme.Styles.Badge.Render(" "+m.theme.Symbols.Arrow+" ")) + 1
}

// renderHelp draws the full-screen key reference: the keys in groups, then the
// two things the keys alone do not explain.
func (m *model) renderHelp() string {
	styles := m.theme.Styles

	heading := func(text string) string {
		return styles.Subtitle.Bold(true).Render(text)
	}
	body := func(lines ...string) string {
		return styles.Value.Render(strings.Join(lines, "\n"))
	}

	sections := []string{
		styles.Badge.Render(" opcua explorer ") + "  " + styles.Faint.Render(m.client.Endpoint),
		"",
		m.help.FullHelpView(m.keys.FullHelp()),
		"",
		heading("The command bar") + "  " + styles.Faint.Render("press :"),
		body(
			"Takes a node id (ns=2;s=Machine), a standard name (Objects, Server, Types),",
			"or a browse path relative to the current node (Server/ServerStatus).",
			"Type :q to quit.",
		),
		"",
		heading("The tree") + "  " + styles.Faint.Render("→ expands, ← collapses, enter re-roots"),
		body(
			"Expanding a node fetches its level once and keeps it, so the shape of the",
			"address space builds up as you walk it. → on an open node steps into it,",
			"← on a closed one steps back out to its parent. Enter makes the selected",
			"node the root, which is how to get past a branch too deep to read.",
		),
		"",
		heading("The filter") + "  " + styles.Faint.Render("press /"),
		body(
			"Narrows the tree as you type, matching browse name, display name and node",
			"id, and keeping any branch whose descendant matches. Enter keeps it;",
			"escape clears it.",
		),
		"",
		heading("Watching") + "  " + styles.Faint.Render("press w on a variable"),
		body(
			"Creates one subscription and adds a monitored item to it, so the values in",
			"the watch panel are pushed by the server rather than polled.",
		),
		"",
		styles.Faint.Render("press any key to go back"),
	}

	// The panel is framed and inset so the help reads as an overlay rather than
	// as the screen it replaced.
	panel := lipgloss.NewStyle().
		Padding(1, 2).
		MaxWidth(m.width).
		Render(strings.Join(sections, "\n"))

	return panel
}

// renderAttributes draws the attribute list of the selected node.
func (m *model) renderAttributes(set cligen.AttributeSet) string {
	styles := m.theme.Styles
	width := m.detailWidth()

	lines := []string{
		styles.Title.Render(render.Truncate(set.NodeID, width)),
		styles.Faint.Render(set.NodeClass),
		"",
	}

	for _, attribute := range set.Attributes {
		value := derefOr(attribute.Text, "")
		if strings.HasPrefix(attribute.Status, "Bad") {
			value = attribute.Status
		}
		lines = append(lines, keyValueLines(m.theme, attribute.Name, value, width)...)
	}

	return strings.Join(lines, "\n")
}

// renderType draws the type description of the selected node.
func (m *model) renderType(info cligen.TypeInfo) string {
	styles := m.theme.Styles
	width := m.detailWidth()

	lines := []string{
		styles.Title.Render(render.Truncate(info.BrowseName, width)),
		styles.Faint.Render(fmt.Sprintf("%s · %s", info.NodeID, info.Kind)),
		"",
	}

	if info.Description != nil && *info.Description != "" {
		lines = append(lines, wrap(styles.Dim, *info.Description, width), "")
	}
	if len(info.SuperTypes) > 0 {
		lines = append(lines, keyValueLines(m.theme, "inherits",
			strings.Join(info.SuperTypes, " "+m.theme.Symbols.Arrow+" "), width)...)
	}
	if len(info.Encodings) > 0 {
		lines = append(lines, keyValueLines(m.theme, "encodings", strings.Join(info.Encodings, ", "), width)...)
	}

	if len(info.Fields) > 0 {
		lines = append(lines, "", styles.Subtitle.Render("fields"))
		for _, field := range info.Fields {
			value := derefOr(field.DataType, "")
			if field.Value != nil {
				value = strconv.Itoa(*field.Value)
			}
			lines = append(lines, keyValueLines(m.theme, field.Name, value, width)...)
		}
	}

	if len(info.SubTypes) > 0 {
		lines = append(lines, "", styles.Subtitle.Render("subtypes"))
		lines = append(lines, wrap(styles.Dim, strings.Join(info.SubTypes, ", "), width))
	}

	return strings.Join(lines, "\n")
}

// renderReferences draws the references of the selected node.
func (m *model) renderReferences(references []cligen.Reference) string {
	styles := m.theme.Styles
	width := m.detailWidth()

	lines := []string{
		styles.Title.Render(fmt.Sprintf("%d reference(s)", len(references))),
		"",
	}

	for _, reference := range references {
		arrow := m.theme.Symbols.Arrow
		if !reference.IsForward {
			arrow = m.theme.Symbols.Inverse
		}
		lines = append(lines,
			fmt.Sprintf("%s %s", styles.Faint.Render(arrow), styles.Subtitle.Render(reference.ReferenceType)),
			"  "+styles.Value.Render(render.Truncate(reference.BrowseName, width-2)),
			"  "+styles.Faint.Render(render.Truncate(reference.NodeID, width-2)),
		)
	}

	return strings.Join(lines, "\n")
}

// renderWatch draws the live watch panel: current value, trend, and how often it
// has changed.
func (m *model) renderWatch() string {
	styles := m.theme.Styles
	width := m.detailWidth()

	if len(m.watchOrder) == 0 {
		return styles.Faint.Render("no watches - press w on a variable")
	}

	lines := []string{
		styles.Title.Render(fmt.Sprintf("%s %d watched", m.theme.Symbols.Live, len(m.watchOrder))),
		"",
	}

	for _, nodeID := range m.watchOrder {
		row := m.watched[nodeID]
		if row == nil {
			continue
		}

		value := styles.Accent.Bold(true).Render(render.Truncate(row.value, width-2))
		if strings.HasPrefix(row.status, "Bad") {
			value = styles.Bad.Render(row.status)
		}

		lines = append(lines,
			styles.Subtitle.Render(render.Truncate(row.name, width)),
			"  "+value,
		)

		if len(row.history) > 1 {
			lines = append(lines, "  "+styles.Good.Render(sparkline(m.theme, row.history, width-4)))
		}

		stamp := ""
		if !row.stamp.IsZero() {
			stamp = row.stamp.Local().Format("15:04:05.000")
		}
		lines = append(lines, "  "+styles.Faint.Render(fmt.Sprintf("%s · %d update(s)", stamp, row.updates)), "")
	}

	return strings.Join(lines, "\n")
}

// sparkline draws a trend, keeping only the most recent points that fit.
func sparkline(theme render.Theme, values []float64, width int) string {
	if width <= 0 {
		return ""
	}
	if len(values) > width {
		values = values[len(values)-width:]
	}

	sparks := theme.Symbols.Sparks
	low, high := values[0], values[0]
	for _, value := range values {
		low = min(low, value)
		high = max(high, value)
	}

	var b strings.Builder
	span := high - low
	for _, value := range values {
		index := len(sparks) / 2
		if span > 0 {
			index = int((value - low) / span * float64(len(sparks)-1))
		}
		b.WriteRune(sparks[index])
	}
	return b.String()
}

// keyValueLines renders one key/value pair for the detail pane, wrapping the
// value under its key when it is too long to sit beside it.
func keyValueLines(theme render.Theme, key, value string, width int) []string {
	styles := theme.Styles
	label := styles.Key.Render(key)

	if lipgloss.Width(key)+lipgloss.Width(value)+2 <= width {
		return []string{label + "  " + styles.Value.Render(value)}
	}
	return []string{label, "  " + wrap(styles.Value, value, width-2)}
}

// wrap renders text wrapped to width in the given style.
func wrap(style lipgloss.Style, text string, width int) string {
	if width <= 0 {
		return style.Render(text)
	}
	return style.Width(width).Render(text)
}

// pad right-pads to width, measured in display cells.
func pad(text string, width int) string {
	gap := width - lipgloss.Width(text)
	if gap <= 0 {
		return text
	}
	return text + strings.Repeat(" ", gap)
}

func derefOr(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}
