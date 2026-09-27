package highlight_test

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
	"github.com/lewtec/patlint/pkg/ui/highlight"
)

func TestLinesPlainSplitsSource(t *testing.T) {
	src := []byte("package main\n\nfunc Hello() {}\n")
	got := highlight.Lines(t.Context(), src, "main.go", highlight.Options{Color: false})
	want := []string{"package main", "", "func Hello() {}", ""}
	require.Equal(t, want, got)
}

func TestLinesColorStylesKeywords(t *testing.T) {
	src := []byte("package main\n")
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	got := highlight.Lines(t.Context(), src, "main.go", highlight.Options{Color: true, Walker: w})
	require.NotEmpty(t, got)
	require.Contains(t, got[0], "\x1b[")

	joined := strings.Join(got, "\n")
	require.Contains(t, joined, "package")
	require.Contains(t, joined, "main")

}

func TestLinesEmpty(t *testing.T) {
	got := highlight.Lines(t.Context(), nil, "main.go", highlight.Options{Color: true})
	require.Nil(t, got)

}
