package sitter

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse_contract(t *testing.T) {
	engines := []struct {
		name string
		e    Engine
		ok   string
	}{
		{name: "stub", e: Stub{}, ok: StubLanguage},
	}
	src := []byte("package p\n")
	for _, tc := range engines {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.e.Has(t.Context(), "") || tc.e.Has(t.Context(), "no-such-lang") {
				t.Fatal("Has empty/unknown")
			}
			if !tc.e.Has(t.Context(), tc.ok) {
				t.Fatalf("Has(%q)=false", tc.ok)
			}
			if _, err := tc.e.Parse(t.Context(), src, ""); !errors.Is(err, ErrUnsupportedLanguage) {
				t.Fatalf("empty language: err=%v", err)
			}
			if _, err := tc.e.Parse(t.Context(), src, "no-such-lang"); !errors.Is(err, ErrUnsupportedLanguage) {
				t.Fatalf("unknown language: err=%v", err)
			}
			tree, err := tc.e.Parse(t.Context(), src, tc.ok)
			if err != nil {
				t.Fatal(err)
			}
			if tree == nil || tree.Root == nil || tree.Root.IsNull() {
				t.Fatal("nil root")
			}
			if tree.Language != tc.ok {
				t.Fatalf("Language=%q want %q", tree.Language, tc.ok)
			}
			if tree.Root.StartByte() != 0 || tree.Root.EndByte() != uint32(len(src)) {
				t.Fatalf("root span [%d,%d) want [0,%d)", tree.Root.StartByte(), tree.Root.EndByte(), len(src))
			}
			walk(tree.Root) // must not panic; Child pointers stay in-tree
		})
	}
}

func TestStub_QueryUnsupported(t *testing.T) {
	t.Parallel()
	if _, err := (Stub{}).Query(t.Context(), nil, StubLanguage, `(ident) @n`); !errors.Is(err, ErrUnsupportedQuery) {
		t.Fatalf("err=%v", err)
	}
}

func TestStub_fields(t *testing.T) {
	t.Parallel()
	src := []byte("foo=bar")
	tree, err := Stub{}.Parse(t.Context(), src, StubLanguage)
	if err != nil {
		t.Fatal(err)
	}
	got := dump(tree.Root)
	want := []step{
		{typ: "source_file", field: "", start: 0, end: 7, named: true, kids: 2},
		{typ: "ident", field: "name", start: 0, end: 3, named: true},
		{typ: "content", field: "body", start: 3, end: 7, named: true},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(step{})); diff != "" {
		t.Fatal(diff)
	}
	if n := tree.Root.NamedChild(0); n == nil || n.Type() != "ident" {
		t.Fatalf("NamedChild(0)=%v", n)
	}
	if tree.Root.NamedChildCount() != 2 {
		t.Fatalf("NamedChildCount=%d", tree.Root.NamedChildCount())
	}
}

type step struct {
	typ   string
	field string
	start uint32
	end   uint32
	named bool
	kids  uint32
}

func dump(n *Node) []step {
	var out []step
	var rec func(*Node, string)
	rec = func(n *Node, field string) {
		if n == nil || n.IsNull() {
			return
		}
		out = append(out, step{
			typ:   n.Type(),
			field: field,
			start: n.StartByte(),
			end:   n.EndByte(),
			named: n.IsNamed(),
			kids:  n.ChildCount(),
		})
		for i := range n.ChildCount() {
			rec(n.Child(i), n.FieldNameForChild(i))
		}
	}
	rec(n, "")
	return out
}

func walk(n *Node) {
	if n == nil || n.IsNull() {
		return
	}
	for i := range n.ChildCount() {
		walk(n.Child(i))
	}
}

func findField(n *Node, field string) *Node {
	if n == nil {
		return nil
	}
	for i := range n.ChildCount() {
		if n.FieldNameForChild(i) == field {
			return n.Child(i)
		}
	}
	return nil
}
