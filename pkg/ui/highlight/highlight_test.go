package highlight_test

import (
	"bytes"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
	"github.com/lewtec/patlint/pkg/ui/highlight"
)

func TestWritePlainCopiesSource(t *testing.T) {
	src := []byte("package main\n\nfunc Hello() {}\n")
	var buf bytes.Buffer
	err := highlight.Write(t.Context(), &buf, src, "main.go", highlight.Options{Color: false})
	require.NoError(t, err)
	require.True(t, bytes.Equal(buf.Bytes(), src),
		"plain write changed source:\n got %q\nwant %q", buf.Bytes(), src)

}

func TestWriteColorStylesKeywords(t *testing.T) {
	src := []byte("package main\n")
	var buf bytes.Buffer
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)
	{

		err := highlight.Write(t.Context(), &buf, src, "main.go", highlight.Options{Color: true, Walker: w})
		require.NoError(t, err)
	}

	out := buf.String()
	// lipgloss / termenv emit ESC sequences when Color is forced.
	require.Contains(t, out, "\x1b[")
	// Visible text still contains the source words.
	require.Contains(t, out, "package")
	require.Contains(t, out, "main")

}

func TestWriteUnsupportedLanguageFallsBack(t *testing.T) {
	src := []byte("just plain text\n")
	var buf bytes.Buffer
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)
	{

		err := highlight.Write(t.Context(), &buf, src, "notes.txt", highlight.Options{Color: true, Walker: w})
		require.NoError(t, err)
	}
	// Unparseable: still returns the full source (with or without spans).
	require.Contains(t, buf.String(), "just plain text")

}

func TestAutoColorRespectsNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	require.False(t, highlight.AutoColor(nil),
		"NO_COLOR set: AutoColor must be false")

}
