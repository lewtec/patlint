package pattern_test

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestBuildTape_Go(t *testing.T) {
	src := []byte("package main\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	cells, lang, err := w.BuildTape(t.Context(), src, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if lang != "go" {
		t.Fatalf("lang=%q", lang)
	}
	if len(cells) == 0 {
		t.Fatal("empty tape")
	}
	for _, c := range cells {
		t.Logf("cell type=%q %d-%d", c.Type, c.StartByte, c.EndByte)
	}
}

func TestBuildTape_AstroFrontmatterEmbed(t *testing.T) {
	src := []byte("---\nconst title = \"hi\"\n---\n<meta charset=\"utf-8\" />\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	cells, lang, err := w.BuildTape(t.Context(), src, "BaseHead.astro")
	if err != nil {
		t.Fatal(err)
	}
	if lang != "astro" {
		t.Fatalf("lang=%q", lang)
	}
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
	if sawOpaqueFront {
		t.Fatal("frontmatter_js_block should be spliced out for embed leaves")
	}
	if !sawConst {
		t.Fatal("expected const keyword cell from typescript embed")
	}
	if !sawStr {
		t.Fatal("expected string cell from typescript embed")
	}
}
