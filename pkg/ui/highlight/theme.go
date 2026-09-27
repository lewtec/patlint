package highlight

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/muesli/termenv"
)

// TokenStyles maps product tok-* classes to lipgloss styles using the
// terminal's ANSI palette (colors 0–15). Those slots follow the user's
// shell/terminal theme rather than fixed RGB values.
//
// Product rule: all terminal TUI (not only highlight) must use this palette
// model — see pkg/ui/COLORS.md and AGENTS.md "Terminal TUI colors".
//
// r may be nil (uses lipgloss default renderer).
func TokenStyles(r *lipgloss.Renderer) map[string]lipgloss.Style {
	if r == nil {
		r = lipgloss.DefaultRenderer()
	}
	// ANSI indices track the theme: 1 red, 2 green, 3 yellow, 4 blue,
	// 5 magenta, 6 cyan, 7 white, 8 bright-black (usually muted).
	return map[string]lipgloss.Style{
		ingest.HLKeyword: r.NewStyle().Foreground(lipgloss.Color("5")),
		ingest.HLString:  r.NewStyle().Foreground(lipgloss.Color("2")),
		ingest.HLNumber:  r.NewStyle().Foreground(lipgloss.Color("3")),
		ingest.HLComment: r.NewStyle().Foreground(lipgloss.Color("8")).Italic(true),
		ingest.HLType:    r.NewStyle().Foreground(lipgloss.Color("6")),
		ingest.HLConst:   r.NewStyle().Foreground(lipgloss.Color("1")),
		ingest.HLOp:      r.NewStyle().Foreground(lipgloss.Color("3")),
		ingest.HLPunct:   r.NewStyle().Foreground(lipgloss.Color("8")),
		// HLIdent / HLOther: default terminal foreground.
	}
}

// EnvColorProfile is the process terminal profile (respects COLORTERM / TERM).
// Palette indices stay theme-native on color terminals. When the env profile
// is Ascii (pipes/tests), returns ANSI so explicit --color=always still emits
// SGR using the 16-color palette (not fixed RGB truecolor).
func EnvColorProfile() termenv.Profile {
	p := termenv.EnvColorProfile()
	if p == termenv.Ascii {
		return termenv.ANSI
	}
	return p
}
