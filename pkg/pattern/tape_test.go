package pattern_test

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestBuildTape_Go(t *testing.T) {
	src := []byte("package main\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	cells, lang, err := w.BuildTape(t.Context(), src, "main.go")
	require.NoError(t, err)
	require.Equal(t, "go", lang,
		"lang=%q", lang)
	require.NotEmpty(t, cells,
		"empty tape")

	for _, c := range cells {
		t.Logf("cell type=%q %d-%d", c.Type, c.StartByte, c.EndByte)
	}
}

func TestBuildTape_AstroFrontmatterEmbed(t *testing.T) {
	src := []byte("---\nconst title = \"hi\"\n---\n<meta charset=\"utf-8\" />\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	cells, lang, err := w.BuildTape(t.Context(), src, "BaseHead.astro")
	require.NoError(t, err)
	require.Equal(t, "astro", lang,
		"lang=%q", lang)

	var sawConst, sawStr, sawOpaqueFront bool
	for _, c := range cells {
		text := string(src[c.StartByte:c.EndByte])
		if c.Type == "frontmatter_js_block" {
			sawOpaqueFront = true
		}
		if text == "const" && c.TokenClass == "tok-kw" {
			sawConst = true
		}
		if strings.Contains(text, "hi") && c.TokenClass == "tok-str" {
			sawStr = true
		}
		t.Logf("type=%q class=%q %q", c.Type, c.TokenClass, text)
	}
	require.False(t, sawOpaqueFront,
		"frontmatter_js_block should be spliced out for embed leaves")
	require.True(t, sawConst,
		"expected const keyword cell from typescript embed")
	require.True(t, sawStr,
		"expected string cell from typescript embed")

}
