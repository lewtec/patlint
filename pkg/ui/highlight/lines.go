package highlight

import (
	"context"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lewtec/patlint/pkg/tape"
)

// Lines splits source into display lines (no trailing newline on each).
// When opts.Color is true, builds the structural tape via extract packs and
// styles cells by TokenClass (same path as Write).
func Lines(ctx context.Context, source []byte, filePath string, opts Options) []string {
	if len(source) == 0 {
		return nil
	}
	if !opts.Color {
		return strings.Split(string(source), "\n")
	}

	if opts.Walker == nil {
		return strings.Split(string(source), "\n")
	}
	cells, _, err := opts.Walker.BuildTape(ctx, source, filePath)
	if err != nil || len(cells) == 0 {
		return strings.Split(string(source), "\n")
	}

	r := lipgloss.DefaultRenderer()
	r.SetColorProfile(EnvColorProfile())
	styles := TokenStyles(r)

	var b strings.Builder
	writeTapeString(&b, source, cells, styles)
	return strings.Split(b.String(), "\n")
}

func writeTapeString(b *strings.Builder, source []byte, cells []tape.Cell, styles map[string]lipgloss.Style) {
	var pos uint32
	for _, c := range cells {
		if c.StartByte > pos {
			b.Write(source[pos:c.StartByte])
		}
		if c.EndByte > uint32(len(source)) || c.StartByte >= c.EndByte {
			continue
		}
		text := string(source[c.StartByte:c.EndByte])
		if c.TokenClass != "" {
			if st, ok := styles[c.TokenClass]; ok {
				text = st.Render(text)
			}
		}
		b.WriteString(text)
		pos = c.EndByte
	}
	if int(pos) < len(source) {
		b.Write(source[pos:])
	}
}
