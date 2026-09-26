package templ_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	_ "github.com/lewtec/patlint/pkg/ingest/go/templ"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestTemplLocalBindings_ParamAndRange(t *testing.T) {
	src := []byte(`package views

templ Hello(name string) {
	<div>{ name }</div>
}

templ List(items []string) {
	for _, item := range items {
		<span>{ item }</span>
	}
}
`)
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, src, "x.templ", "templ")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	idx := ingest.LocalBindingsForLanguage(t.Context(), vm, project.NewSession(".").WithEngine(ccgo.Engine{}), pf.Root, src, "templ", "x.templ")
	if idx == nil {
		t.Fatal("nil index")
	}

	// Find spans for name param and use, item range var and use.
	mustBind := func(label string, start, end uint32, wantDef bool) {
		t.Helper()
		sp := ingestutil.Span{StartByte: start, EndByte: end}
		site, ok := idx.Lookup(sp)
		if !ok {
			t.Fatalf("%s: no binding at %d-%d text=%q", label, start, end, src[start:end])
		}
		if site.IsDef != wantDef {
			t.Fatalf("%s: IsDef=%v want %v", label, site.IsDef, wantDef)
		}
		if site.Name == "" {
			t.Fatalf("%s: empty name", label)
		}
	}

	// Brute-force: scan for identifier nodes via index keys
	foundNameDef, foundNameUse := false, false
	foundItemDef, foundItemUse := false, false
	for sp, site := range idx.BySpan {
		text := string(src[sp.StartByte:sp.EndByte])
		switch {
		case text == "name" && site.IsDef:
			foundNameDef = true
			mustBind("name def", sp.StartByte, sp.EndByte, true)
		case text == "name" && !site.IsDef:
			foundNameUse = true
		case text == "item" && site.IsDef:
			foundItemDef = true
		case text == "item" && !site.IsDef:
			foundItemUse = true
		case text == "items" && !site.IsDef:
			// range RHS use of param
			if site.Name != "items" {
				t.Fatalf("items use name=%q", site.Name)
			}
		}
	}
	if !foundNameDef || !foundNameUse {
		t.Fatalf("name def/use missing def=%v use=%v (n=%d)", foundNameDef, foundNameUse, len(idx.BySpan))
	}
	if !foundItemDef || !foundItemUse {
		t.Fatalf("item def/use missing def=%v use=%v", foundItemDef, foundItemUse)
	}
}

func TestTemplAbstractUnitTypes(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	types := vm.ScopeNodeTypes("templ")
	want := map[string]bool{"component_declaration": true, "function_declaration": true}
	for w := range want {
		found := false
		for _, tpe := range types {
			if tpe == w {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s in %v", w, types)
		}
	}
}
