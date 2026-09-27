package treesitter

import (
	"testing"

	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/stretchr/testify/require"
)

func TestParse_contract(t *testing.T) {
	t.Parallel()
	e := Engine{}
	src := []byte("package p\n")
	require.False(t, e.Has(t.Context(), "") || e.Has(t.Context(), "no-such-lang"),
		"Has empty/unknown")
	require.True(t, e.Has(t.Context(), "go"),
		"Has(go)=false")
	{

		_, err := e.Parse(t.Context(), src, "")
		require.ErrorIs(t, err, sitter.ErrUnsupportedLanguage,
			"empty language: err=%v", err)
	}
	{

		_, err := e.Parse(t.Context(), src, "no-such-lang")
		require.ErrorIs(t, err, sitter.ErrUnsupportedLanguage,
			"unknown language: err=%v", err)
	}

	tree, err := e.Parse(t.Context(), src, "go")
	require.NoError(t, err)
	require.False(t, tree == nil || tree.Root == nil || tree.Root.IsNull(),
		"nil root")
	require.Equal(t, "go", tree.Language,
		"Language=%q", tree.Language)

}

func TestParse_goFuncFields(t *testing.T) {
	t.Parallel()
	src := []byte("package p\nfunc f() {}\n")
	tree, err := Engine{}.Parse(t.Context(), src, "go")
	require.NoError(t, err)
	require.Equal(t, "source_file", tree.Root.Type(),
		"root type %q", tree.Root.Type())

	var fn *sitter.Node
	for i := range tree.Root.ChildCount() {
		ch := tree.Root.Child(i)
		if ch != nil && ch.Type() == "function_declaration" {
			fn = ch
			break
		}
	}
	require.NotNil(t, fn,
		"no function_declaration")

	var name *sitter.Node
	for i := range fn.ChildCount() {
		if fn.FieldNameForChild(i) == "name" {
			name = fn.Child(i)
			break
		}
	}
	require.False(t, name == nil || name.Type() != "identifier",
		"func name=%v", name)
	require.Equal(t, "f", string(src[name.StartByte():name.EndByte()]),
		"name text %q", src[name.StartByte():name.EndByte()])

}

func TestQuery_goFuncName(t *testing.T) {
	t.Parallel()
	src := []byte("package p\nfunc f() {}\n")
	matches, err := Engine{}.Query(t.Context(), src, "go", `(function_declaration name: (identifier) @name)`)
	require.NoError(t, err)
	require.Len(t, matches, 1,
		"matches=%d", len(matches))
	require.False(t, len(matches[0].Captures) != 1 || matches[0].Captures[0].Text != "f",
		"captures=%v", matches[0].Captures)

}
