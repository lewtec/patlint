// Package highlight renders source to the terminal with structural-tape
// syntax colors (same tok-* classes as the web code view).
//
// Product path: extract packs (rft) → structural tape (cells carry TokenClass) → color.
package highlight

import (
	"context"
	"io"
	"os"

	"github.com/lewtec/patlint/pkg/walker"

	"github.com/charmbracelet/lipgloss"
	"github.com/lewtec/patlint/pkg/tape"
	"github.com/mattn/go-isatty"
)

// Options controls terminal rendering.
type Options struct {
	// Color enables ANSI styling. When false, source is written unchanged.
	Color bool
	// Walker attributes the file (VM PackQueries). Required when Color is true.
	Walker *walker.Walker
}

// AutoColor reports whether colored output should be used for w. Honors
// NO_COLOR. Non-file writers (buffers) return false.
func AutoColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok || f == nil {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// Write prints source for filePath. When opts.Color is true, builds the
// structural tape via extract packs and styles cells by TokenClass.
// On pack/parse failure the raw source is written (no error).
func Write(ctx context.Context, w io.Writer, source []byte, filePath string, opts Options) error {
	if len(source) == 0 {
		return nil
	}
	if !opts.Color {
		_, err := w.Write(source)
		return err
	}

	if opts.Walker == nil {
		_, err := w.Write(source)
		return err
	}
	cells, _, err := opts.Walker.BuildTape(ctx, source, filePath)
	if err != nil || len(cells) == 0 {
		_, werr := w.Write(source)
		return werr
	}

	r := lipgloss.NewRenderer(w)
	r.SetColorProfile(EnvColorProfile())
	styles := TokenStyles(r)
	return writeTape(w, source, cells, styles)
}

func writeTape(w io.Writer, source []byte, cells []tape.Cell, styles map[string]lipgloss.Style) error {
	var pos uint32
	for _, c := range cells {
		if c.StartByte > pos {
			if _, err := w.Write(source[pos:c.StartByte]); err != nil {
				return err
			}
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
		if _, err := io.WriteString(w, text); err != nil {
			return err
		}
		pos = c.EndByte
	}
	if int(pos) < len(source) {
		_, err := w.Write(source[pos:])
		return err
	}
	return nil
}
