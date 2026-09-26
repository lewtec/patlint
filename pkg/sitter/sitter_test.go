package sitter

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
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
			require.False(t, tc.e.Has(t.Context(), "") || tc.e.Has(t.Context(), "no-such-lang"),
				"Has empty/unknown")
			require.True(t, tc.e.Has(t.Context(), tc.ok),
				"Has(%q)=false", tc.ok)
			{

				_, err := tc.e.Parse(t.Context(), src, "")
				require.ErrorIs(t, err, ErrUnsupportedLanguage,
					"empty language: err=%v", err)
			}
			{

				_, err := tc.e.Parse(t.Context(), src, "no-such-lang")
				require.ErrorIs(t, err, ErrUnsupportedLanguage,
					"unknown language: err=%v", err)
			}

			tree, err := tc.e.Parse(t.Context(), src, tc.ok)
			require.NoError(t, err)
			require.False(t, tree == nil || tree.Root == nil || tree.Root.IsNull(),
				"nil root")
			require.Equal(t, tc.ok, tree.Language,
				"Language=%q want %q", tree.Language, tc.ok)
			require.False(t, tree.Root.StartByte() != 0 || tree.Root.EndByte() != uint32(len(src)),
				"root span [%d,%d) want [0,%d)", tree.Root.StartByte(), tree.Root.EndByte(), len(src))

			walk(tree.Root) // must not panic; Child pointers stay in-tree
		})
	}
}

func TestStub_QueryUnsupported(t *testing.T) {
	t.Parallel()
	_, err := (Stub{}).Query(t.Context(), nil, StubLanguage, `(ident) @n`)
	require.ErrorIs(t, err, ErrUnsupportedQuery,
		"err=%v", err)

}

func TestStub_fields(t *testing.T) {
	t.Parallel()
	src := []byte("foo=bar")
	tree, err := Stub{}.Parse(t.Context(), src, StubLanguage)
	require.NoError(t, err)

	got := dump(tree.Root)
	want := []step{
		{typ: "source_file", field: "", start: 0, end: 7, named: true, kids: 2},
		{typ: "ident", field: "name", start: 0, end: 3, named: true},
		{typ: "content", field: "body", start: 3, end: 7, named: true},
	}
	diff := cmp.Diff(want, got, cmp.AllowUnexported(step{}))
	require.Empty(t, diff)

	n := tree.Root.NamedChild(0)
	require.False(t, n == nil || n.Type() != "ident",
		"NamedChild(0)=%v", n)
	require.Equal(t, uint32(2), tree.Root.NamedChildCount())

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
