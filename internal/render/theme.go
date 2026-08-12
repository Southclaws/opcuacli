// Package render turns OPC UA results into terminal output: themed styles, a
// table and tree renderer, key/value blocks, sparklines, and the status-code
// colouring that runs through all of them.
package render

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/charmtone"
)

// Palette is the full set of colours a theme provides. Every style in the tool
// is built from these, so a new theme only has to supply colours.
type Palette struct {
	Text    color.Color // primary foreground
	Dim     color.Color // secondary foreground, still readable
	Faint   color.Color // decoration: borders, separators, punctuation
	Accent  color.Color // the theme's signature colour
	Accent2 color.Color // secondary highlight, for a second dimension of data
	Accent3 color.Color // tertiary highlight, for a third

	Good      color.Color // a Good status code
	Uncertain color.Color // an Uncertain status code
	Bad       color.Color // a Bad status code

	SelBg color.Color // selected row background
	SelFg color.Color // selected row foreground
	Panel color.Color // panel background, in the TUI
}

// Styles are the styles every renderer draws with, derived from a Palette.
type Styles struct {
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Key      lipgloss.Style
	Value    lipgloss.Style
	Dim      lipgloss.Style
	Faint    lipgloss.Style
	Accent   lipgloss.Style

	Good      lipgloss.Style
	Uncertain lipgloss.Style
	Bad       lipgloss.Style

	NodeID lipgloss.Style
	Path   lipgloss.Style
	Class  lipgloss.Style
	Type   lipgloss.Style

	Header   lipgloss.Style
	Cell     lipgloss.Style
	Selected lipgloss.Style
	Border   lipgloss.Style
	Badge    lipgloss.Style
	Panel    lipgloss.Style
}

// Theme is a named palette together with the styles derived from it.
type Theme struct {
	Name    string
	Dark    bool
	Colors  Palette
	Styles  Styles
	Symbols Symbols
}

// Symbols are the glyphs used to draw structure and state. They are collected
// here so an ASCII-only terminal can be served by swapping the set rather than
// by threading a flag through every renderer.
type Symbols struct {
	Good      string
	Uncertain string
	Bad       string
	Arrow     string
	Inverse   string
	Bullet    string
	Ellipsis  string
	Live      string
	Sparks    []rune
}

func unicodeSymbols() Symbols {
	return Symbols{
		Good:      "●",
		Uncertain: "◐",
		Bad:       "✗",
		Arrow:     "→",
		Inverse:   "←",
		Bullet:    "•",
		Ellipsis:  "…",
		Live:      "◉",
		Sparks:    []rune("▁▂▃▄▅▆▇█"),
	}
}

// ThemeNames are the themes a user may select, in the order they are offered.
var ThemeNames = []string{"charm", "nord", "dracula", "mono"}

// ThemeByName returns the named theme, or the charm theme when the name is not
// recognised. dark selects the variant tuned for a dark background.
func ThemeByName(name string, dark bool) Theme {
	var p Palette
	switch name {
	case "nord":
		p = nordPalette(dark)
	case "dracula":
		p = draculaPalette()
	case "mono":
		p = monoPalette(dark)
	default:
		name = "charm"
		p = charmPalette(dark)
	}

	return Theme{Name: name, Dark: dark, Colors: p, Styles: newStyles(p), Symbols: unicodeSymbols()}
}

// charmPalette draws from CharmTone, the palette the charm libraries ship, so
// the tool looks like the toolkit it is built with.
func charmPalette(dark bool) Palette {
	pick := lipgloss.LightDark(dark)
	hex := func(k charmtone.Key) color.Color { return lipgloss.Color(k.Hex()) }

	return Palette{
		Text:      pick(hex(charmtone.Pepper), hex(charmtone.Salt)),
		Dim:       pick(hex(charmtone.Charcoal), hex(charmtone.Smoke)),
		Faint:     pick(hex(charmtone.Squid), hex(charmtone.Iron)),
		Accent:    hex(charmtone.Charple),
		Accent2:   hex(charmtone.Malibu),
		Accent3:   hex(charmtone.Cheeky),
		Good:      hex(charmtone.Guac),
		Uncertain: hex(charmtone.Zest),
		Bad:       hex(charmtone.Sriracha),
		SelBg:     hex(charmtone.Charple),
		SelFg:     hex(charmtone.Salt),
		Panel:     pick(hex(charmtone.Salt), hex(charmtone.Pepper)),
	}
}

func nordPalette(dark bool) Palette {
	pick := lipgloss.LightDark(dark)
	return Palette{
		Text:      pick(lipgloss.Color("#2E3440"), lipgloss.Color("#ECEFF4")),
		Dim:       pick(lipgloss.Color("#4C566A"), lipgloss.Color("#D8DEE9")),
		Faint:     lipgloss.Color("#616E88"),
		Accent:    lipgloss.Color("#88C0D0"),
		Accent2:   lipgloss.Color("#81A1C1"),
		Accent3:   lipgloss.Color("#B48EAD"),
		Good:      lipgloss.Color("#A3BE8C"),
		Uncertain: lipgloss.Color("#EBCB8B"),
		Bad:       lipgloss.Color("#BF616A"),
		SelBg:     lipgloss.Color("#434C5E"),
		SelFg:     lipgloss.Color("#ECEFF4"),
		Panel:     pick(lipgloss.Color("#ECEFF4"), lipgloss.Color("#2E3440")),
	}
}

func draculaPalette() Palette {
	return Palette{
		Text:      lipgloss.Color("#F8F8F2"),
		Dim:       lipgloss.Color("#BFBFD0"),
		Faint:     lipgloss.Color("#6272A4"),
		Accent:    lipgloss.Color("#BD93F9"),
		Accent2:   lipgloss.Color("#8BE9FD"),
		Accent3:   lipgloss.Color("#FF79C6"),
		Good:      lipgloss.Color("#50FA7B"),
		Uncertain: lipgloss.Color("#F1FA8C"),
		Bad:       lipgloss.Color("#FF5555"),
		SelBg:     lipgloss.Color("#44475A"),
		SelFg:     lipgloss.Color("#F8F8F2"),
		Panel:     lipgloss.Color("#282A36"),
	}
}

// monoPalette keeps the shape of the output - bold headers, dim decoration,
// reverse-video selection - without using hue to carry meaning.
func monoPalette(dark bool) Palette {
	pick := lipgloss.LightDark(dark)
	text := pick(lipgloss.Color("#000000"), lipgloss.Color("#FFFFFF"))
	return Palette{
		Text:      text,
		Dim:       pick(lipgloss.Color("#4A4A4A"), lipgloss.Color("#BBBBBB")),
		Faint:     lipgloss.Color("#777777"),
		Accent:    text,
		Accent2:   pick(lipgloss.Color("#333333"), lipgloss.Color("#DDDDDD")),
		Accent3:   pick(lipgloss.Color("#555555"), lipgloss.Color("#AAAAAA")),
		Good:      text,
		Uncertain: pick(lipgloss.Color("#555555"), lipgloss.Color("#AAAAAA")),
		Bad:       text,
		SelBg:     pick(lipgloss.Color("#DDDDDD"), lipgloss.Color("#444444")),
		SelFg:     text,
		Panel:     pick(lipgloss.Color("#FFFFFF"), lipgloss.Color("#000000")),
	}
}

func newStyles(p Palette) Styles {
	base := lipgloss.NewStyle()
	return Styles{
		Title:    base.Bold(true).Foreground(p.Accent),
		Subtitle: base.Foreground(p.Accent2),
		Key:      base.Foreground(p.Dim),
		Value:    base.Foreground(p.Text),
		Dim:      base.Foreground(p.Dim),
		Faint:    base.Foreground(p.Faint),
		Accent:   base.Foreground(p.Accent),

		Good:      base.Foreground(p.Good),
		Uncertain: base.Foreground(p.Uncertain),
		Bad:       base.Foreground(p.Bad),

		NodeID: base.Foreground(p.Accent2),
		Path:   base.Foreground(p.Dim),
		Class:  base.Foreground(p.Accent3),
		Type:   base.Foreground(p.Faint),

		Header:   base.Bold(true).Foreground(p.Accent).Padding(0, 1),
		Cell:     base.Foreground(p.Text).Padding(0, 1),
		Selected: base.Foreground(p.SelFg).Background(p.SelBg),
		Border:   base.Foreground(p.Faint),
		Badge:    base.Foreground(p.SelFg).Background(p.Accent).Padding(0, 1),
		Panel:    base.Background(p.Panel),
	}
}
