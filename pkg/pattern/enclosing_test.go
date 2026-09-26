package pattern

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestEnclosingFieldArenaMatchesGrammar(t *testing.T) {
	src := []byte("package p\n\nfunc Foo() {\n\tBar()\n}\n\nfunc Bar() {\n\tFoo()\n}\n")
	pf, err := ingestutil.ParseSource(t.Context(), ccgo.Engine{}, src, "x.go", "go")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pf.Close)
	want := map[string]struct{}{"function_declaration": {}}
	idx := buildNodeIndex(pf.Root, want)
	if idx == nil || !idx.ok {
		t.Fatal("want node index")
	}
	for _, name := range []string{"Bar()", "Foo()"} {
		i := strings.Index(string(src), name)
		if i < 0 {
			t.Fatalf("missing %q", name)
		}
		start, end := uint32(i), uint32(i+3)
		got := enclosingNodeField(pf.Root, start, end, "function_declaration", "name", src, idx)
		old := enclosingNodeField(pf.Root, start, end, "function_declaration", "name", src, nil)
		if got != old {
			t.Fatalf("%s: arena %q grammar %q", name, got, old)
		}
		if got == "" {
			t.Fatalf("%s: empty scope", name)
		}
	}
}

func TestEnclosingFieldArenaPeelsCDeclarator(t *testing.T) {
	src := []byte("void main() {\n  helper();\n}\n")
	pf, err := ingestutil.ParseSource(t.Context(), ccgo.Engine{}, src, "a.c", "c")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pf.Close)
	want := map[string]struct{}{"function_definition": {}}
	idx := buildNodeIndex(pf.Root, want)
	if idx == nil || !idx.ok {
		t.Fatal("want node index")
	}
	i := strings.Index(string(src), "helper")
	if i < 0 {
		t.Fatal("missing helper")
	}
	start, end := uint32(i), uint32(i+len("helper"))
	got := enclosingNodeField(pf.Root, start, end, "function_definition", "name", src, idx)
	old := enclosingNodeField(pf.Root, start, end, "function_definition", "name", src, nil)
	if got != old {
		t.Fatalf("arena %q grammar %q", got, old)
	}
	if got != "main" {
		t.Fatalf("scope=%q want main", got)
	}
}
