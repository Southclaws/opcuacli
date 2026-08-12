package render

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"charm.land/lipgloss/v2/tree"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
)

// Printer writes themed output to one stream. Styling is always applied and the
// colour profile of the underlying writer decides what survives, so a piped or
// redirected stream degrades to plain text without the callers knowing.
type Printer struct {
	w     *colorprofile.Writer
	raw   io.Writer
	T     Theme
	Width int

	// Height is the detected terminal height, or 0 when the stream is not a
	// terminal. A live renderer uses it to decide how much it can draw.
	Height int
	// TTY reports whether the stream is an interactive terminal, which decides
	// whether output can be redrawn in place.
	TTY bool
}

// Options configure a Printer.
type Options struct {
	// NoColor forces plain text even on a capable terminal.
	NoColor bool
	// Theme names the palette to use; an unknown name falls back to charm.
	Theme string
	// Width overrides the detected terminal width.
	Width int
}

// NewPrinter builds a Printer for out. When out is the process's stdout its
// terminal capabilities and width are detected; otherwise a conservative
// 100-column plain-text profile is used.
func NewPrinter(out io.Writer, opts Options) *Printer {
	writer := colorprofile.NewWriter(out, os.Environ())
	if opts.NoColor {
		writer.Profile = colorprofile.NoTTY
	}

	width, height := opts.Width, 0
	dark, tty := true, false
	if file, ok := out.(*os.File); ok && term.IsTerminal(file.Fd()) {
		tty = true
		if w, h, err := term.GetSize(file.Fd()); err == nil {
			if w > 0 && width == 0 {
				width = w
			}
			height = h
		}
		// Querying the terminal for its background colour costs a round trip
		// and only pays off when styling is actually going to be emitted.
		if writer.Profile != colorprofile.NoTTY {
			dark = lipgloss.HasDarkBackground(os.Stdin, file)
		}
	}
	if width <= 0 {
		width = 100
	}

	return &Printer{
		w:      writer,
		raw:    out,
		T:      ThemeByName(opts.Theme, dark),
		Width:  width,
		Height: height,
		TTY:    tty,
	}
}

// Writer returns the styled writer, for callers that render their own output.
func (p *Printer) Writer() io.Writer { return p.w }

// Raw returns the unwrapped writer, for machine-readable output that must not
// be touched by the colour profile.
func (p *Printer) Raw() io.Writer { return p.raw }

// Print writes a pre-styled string.
func (p *Printer) Print(s string) { fmt.Fprint(p.w, s) }

// Printf writes a formatted, pre-styled string.
func (p *Printer) Printf(format string, args ...any) { fmt.Fprintf(p.w, format, args...) }

// Println writes a pre-styled string and a newline.
func (p *Printer) Println(args ...any) { fmt.Fprintln(p.w, args...) }

// Title writes a heading with an optional subtitle to its right.
func (p *Printer) Title(title, subtitle string) {
	line := p.T.Styles.Title.Render(title)
	if subtitle != "" {
		line += "  " + p.T.Styles.Faint.Render(subtitle)
	}
	fmt.Fprintln(p.w, line)
}

// Notef writes an informational line prefixed with a bullet.
func (p *Printer) Notef(format string, args ...any) {
	fmt.Fprintf(p.w, "%s %s\n",
		p.T.Styles.Accent.Render(p.T.Symbols.Bullet),
		p.T.Styles.Dim.Render(fmt.Sprintf(format, args...)))
}

// Warnf writes a warning line to the same stream as the output it annotates.
func (p *Printer) Warnf(format string, args ...any) {
	fmt.Fprintf(p.w, "%s %s\n",
		p.T.Styles.Uncertain.Render(p.T.Symbols.Uncertain),
		p.T.Styles.Dim.Render(fmt.Sprintf(format, args...)))
}

// Okf writes a success line.
func (p *Printer) Okf(format string, args ...any) {
	fmt.Fprintf(p.w, "%s %s\n",
		p.T.Styles.Good.Render(p.T.Symbols.Good),
		p.T.Styles.Value.Render(fmt.Sprintf(format, args...)))
}

// Field is one row of a key/value block.
type Field struct {
	Key   string
	Value string
	// Style overrides the value style; nil uses the theme's value style.
	Style *lipgloss.Style
}

// F builds a plain field.
func F(key, value string) Field { return Field{Key: key, Value: value} }

// FS builds a field whose value carries its own style.
func FS(key, value string, style lipgloss.Style) Field {
	return Field{Key: key, Value: value, Style: &style}
}

// KV writes a key/value block with the keys right-aligned into one column, the
// shape used by every `info`-style command.
func (p *Printer) KV(fields []Field) {
	width := 0
	for _, f := range fields {
		if f.Key != "" && lipgloss.Width(f.Key) > width {
			width = lipgloss.Width(f.Key)
		}
	}

	key := p.T.Styles.Key.Width(width + 1).AlignHorizontal(lipgloss.Right)
	for _, f := range fields {
		if f.Key == "" && f.Value == "" {
			fmt.Fprintln(p.w)
			continue
		}
		value := p.T.Styles.Value
		if f.Style != nil {
			value = *f.Style
		}
		fmt.Fprintf(p.w, "%s  %s\n", key.Render(f.Key), value.Render(f.Value))
	}
}

// Table writes a table with a styled header and rows. Column widths are chosen
// by lipgloss; the table is capped to the printer's width so a long value
// wraps inside its cell instead of breaking the layout.
func (p *Printer) Table(headers []string, rows [][]string) {
	if len(rows) == 0 {
		fmt.Fprintln(p.w, p.T.Styles.Faint.Render("no results"))
		return
	}

	t := table.New().
		Headers(headers...).
		Rows(rows...).
		Border(lipgloss.NormalBorder()).
		BorderStyle(p.T.Styles.Border).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false).
		BorderRow(false).
		Wrap(true).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return p.T.Styles.Header
			}
			return p.T.Styles.Cell
		})

	// A fixed width pads every column out to fill the terminal, which reads as
	// sprawl on a narrow result. Only constrain the table when its natural width
	// would overflow, and let it size to its content otherwise.
	if naturalWidth(headers, rows) > p.Width {
		t = t.Width(p.Width)
	}

	fmt.Fprintln(p.w, t.Render())
}

// naturalWidth is the width a table needs to show every cell in full, including
// the single space of padding either side of each one.
func naturalWidth(headers []string, rows [][]string) int {
	columns := make([]int, len(headers))
	for index, header := range headers {
		columns[index] = lipgloss.Width(header)
	}
	for _, row := range rows {
		for index, cell := range row {
			if index < len(columns) {
				columns[index] = max(columns[index], lipgloss.Width(cell))
			}
		}
	}

	total := 0
	for _, width := range columns {
		total += width + 2
	}
	return total
}

// Plain writes tab-separated rows with no styling and no header, the shape that
// pipes cleanly into cut, awk and xargs.
func (p *Printer) Plain(rows [][]string) {
	for _, row := range rows {
		fmt.Fprintln(p.raw, strings.Join(row, "\t"))
	}
}

// TreeNode is one node of a renderable tree. Label carries its own styling.
type TreeNode struct {
	Label    string
	Children []TreeNode
}

// Tree writes an indented tree rooted at a styled label.
func (p *Printer) Tree(root string, children []TreeNode) {
	t := tree.Root(root).
		EnumeratorStyle(p.T.Styles.Border).
		IndenterStyle(p.T.Styles.Border)
	for _, child := range children {
		t.Child(buildTree(child))
	}
	fmt.Fprintln(p.w, t.String())
}

func buildTree(node TreeNode) any {
	if len(node.Children) == 0 {
		return node.Label
	}
	t := tree.Root(node.Label)
	for _, child := range node.Children {
		t.Child(buildTree(child))
	}
	return t
}

// Sparkline renders values as a single line of block glyphs, scaled to the
// range present in the data. Fewer than two points has no shape to draw.
func (p *Printer) Sparkline(values []float64) string {
	sparks := p.T.Symbols.Sparks
	if len(values) == 0 || len(sparks) == 0 {
		return ""
	}

	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	var b strings.Builder
	span := max - min
	for _, v := range values {
		index := 0
		if span > 0 {
			index = int((v - min) / span * float64(len(sparks)-1))
		} else {
			index = len(sparks) / 2
		}
		b.WriteRune(sparks[index])
	}
	return b.String()
}

// Truncate shortens s to width, marking the cut with an ellipsis. Width is
// measured in display cells, so wide glyphs and combining marks are counted as
// the terminal counts them.
func Truncate(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}

	runes := []rune(s)
	for len(runes) > 0 {
		candidate := string(runes) + "…"
		if lipgloss.Width(candidate) <= width {
			return candidate
		}
		runes = runes[:len(runes)-1]
	}
	return "…"
}
