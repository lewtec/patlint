package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestHostOnlyPackClaimsPath(t *testing.T) {
	src := `(under (path "**/*.rft") (as-language "commonlisp"))`
	prog, err := pattern.LoadExtractPack("language_rft.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	if prog.Language != "commonlisp" {
		t.Fatalf("Language=%q", prog.Language)
	}
	if len(prog.Actions) == 0 {
		t.Fatal("host-only pack must emit a path claim action")
	}
	found := false
	for _, a := range prog.Actions {
		if a.HostLang == "commonlisp" && a.Matcher == nil {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want Matcher-nil host claim, got %+v", prog.Actions)
	}
}

func TestProductClaimsRftAndLoadsCommonPaint(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	p := vm.Packs().Program()
	if p == nil {
		t.Fatal("nil program")
	}
	lang, ok := vm.HostLanguage("demo.rft")
	if !ok || lang != "commonlisp" {
		t.Fatalf("demo.rft -> %q ok=%v", lang, ok)
	}
	var paint int
	for _, a := range p.Actions {
		if a.Kind == pattern.ExtractPaint {
			paint++
		}
	}
	if paint == 0 {
		t.Fatal("program has no paint actions (language_common/highlight not loaded?)")
	}
	src := []byte("(under (path \"**/*.go\") (as-language \"go\"))\n")
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	cells, gotLang, err := w.BuildTape(t.Context(), src, "demo.rft")
	if err != nil {
		t.Fatal(err)
	}
	if gotLang != "commonlisp" {
		t.Fatalf("lang=%q", gotLang)
	}
	if len(cells) == 0 {
		t.Fatal("empty tape")
	}
}
