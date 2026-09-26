package ccgo

import (
	"errors"
	"testing"

	"github.com/lewtec/patlint/pkg/sitter"
)

func TestParse_contract(t *testing.T) {
	t.Parallel()
	e := Engine{}
	src := []byte("package p\n")
	if e.Has("") || e.Has("no-such-lang") {
		t.Fatal("Has empty/unknown")
	}
	if !e.Has("go") {
		t.Fatal("Has(go)=false")
	}
	if _, err := e.Parse(src, ""); !errors.Is(err, sitter.ErrUnsupportedLanguage) {
		t.Fatalf("empty language: err=%v", err)
	}
	if _, err := e.Parse(src, "no-such-lang"); !errors.Is(err, sitter.ErrUnsupportedLanguage) {
		t.Fatalf("unknown language: err=%v", err)
	}
	tree, err := e.Parse(src, "go")
	if err != nil {
		t.Fatal(err)
	}
	if tree == nil || tree.Root == nil || tree.Root.IsNull() {
		t.Fatal("nil root")
	}
	if tree.Language != "go" {
		t.Fatalf("Language=%q", tree.Language)
	}
}

func TestParse_goFuncFields(t *testing.T) {
	t.Parallel()
	src := []byte("package p\nfunc f() {}\n")
	tree, err := Engine{}.Parse(src, "go")
	if err != nil {
		t.Fatal(err)
	}
	if tree.Root.Type() != "source_file" {
		t.Fatalf("root type %q", tree.Root.Type())
	}
	var fn *sitter.Node
	for i := range tree.Root.ChildCount() {
		ch := tree.Root.Child(i)
		if ch != nil && ch.Type() == "function_declaration" {
			fn = ch
			break
		}
	}
	if fn == nil {
		t.Fatal("no function_declaration")
	}
	var name *sitter.Node
	for i := range fn.ChildCount() {
		if fn.FieldNameForChild(i) == "name" {
			name = fn.Child(i)
			break
		}
	}
	if name == nil || name.Type() != "identifier" {
		t.Fatalf("func name=%v", name)
	}
	if string(src[name.StartByte():name.EndByte()]) != "f" {
		t.Fatalf("name text %q", src[name.StartByte():name.EndByte()])
	}
}

func TestQuery_goFuncName(t *testing.T) {
	t.Parallel()
	src := []byte("package p\nfunc f() {}\n")
	matches, err := Engine{}.Query(src, "go", `(function_declaration name: (identifier) @name)`)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches=%d", len(matches))
	}
	if len(matches[0].Captures) != 1 || matches[0].Captures[0].Text != "f" {
		t.Fatalf("captures=%v", matches[0].Captures)
	}
}
