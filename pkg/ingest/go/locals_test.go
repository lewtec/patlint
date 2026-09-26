package ingestgo_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestLocalBindings_ParamUse(t *testing.T) {
	src := []byte(`package p
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
`)
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, src, "x.go", "go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	idx := ingest.LocalBindingsForLanguage(t.Context(), vm, project.NewSession(".").WithEngine(ccgo.Engine{}), pf.Root, src, "go", "x.go")
	if idx == nil {
		t.Fatal("nil index")
	}
	// Find def of "a" in params and a use in body
	var defA, useA ingestutil.Span
	for sp, site := range idx.BySpan {
		if site.Name != "a" {
			continue
		}
		if site.IsDef {
			defA = sp
		} else if useA.Empty() {
			useA = sp
		}
	}
	if defA.Empty() {
		t.Fatalf("no def for a; sites=%d", len(idx.BySpan))
	}
	if useA.Empty() {
		t.Fatalf("no use for a; sites=%d", len(idx.BySpan))
	}
	site, ok := idx.Lookup(useA)
	if !ok || site.DefSpan != defA {
		t.Fatalf("use -> def: ok=%v site=%+v defA=%v", ok, site, defA)
	}
	// O(1) map: every site has entry
	if len(idx.BySpan) < 4 { // a,b defs + uses
		t.Fatalf("too few sites: %d", len(idx.BySpan))
	}
}
