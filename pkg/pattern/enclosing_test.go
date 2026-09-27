package pattern

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
	"github.com/stretchr/testify/require"
)

func TestEnclosingFieldArenaMatchesGrammar(t *testing.T) {
	src := []byte("package p\n\nfunc Foo() {\n\tBar()\n}\n\nfunc Bar() {\n\tFoo()\n}\n")
	pf, err := ingestutil.ParseSource(t.Context(), treesitter.Engine{}, src, "x.go", "go")
	require.NoError(t, err)

	t.Cleanup(pf.Close)
	want := map[string]struct{}{"function_declaration": {}}
	idx := buildNodeIndex(pf.Root, want)
	require.NotNil(t, idx)
	require.True(t, idx.ok, "want node index")

	for _, name := range []string{"Bar()", "Foo()"} {
		i := strings.Index(string(src), name)
		require.GreaterOrEqual(t, i, 0, "missing %q", name)

		start, end := uint32(i), uint32(i+3)
		got := enclosingNodeField(pf.Root, start, end, "function_declaration", "name", src, idx)
		old := enclosingNodeField(pf.Root, start, end, "function_declaration", "name", src, nil)
		require.Equal(t, old, got, name)
		require.NotEmpty(t, got, "%s: empty scope", name)

	}
}

func TestEnclosingFieldArenaPeelsCDeclarator(t *testing.T) {
	src := []byte("void main() {\n  helper();\n}\n")
	pf, err := ingestutil.ParseSource(t.Context(), treesitter.Engine{}, src, "a.c", "c")
	require.NoError(t, err)

	t.Cleanup(pf.Close)
	want := map[string]struct{}{"function_definition": {}}
	idx := buildNodeIndex(pf.Root, want)
	require.NotNil(t, idx)
	require.True(t, idx.ok, "want node index")

	i := strings.Index(string(src), "helper")
	require.GreaterOrEqual(t, i, 0, "missing helper")

	start, end := uint32(i), uint32(i+len("helper"))
	got := enclosingNodeField(pf.Root, start, end, "function_definition", "name", src, idx)
	old := enclosingNodeField(pf.Root, start, end, "function_definition", "name", src, nil)
	require.Equal(t, old, got)
	require.Equal(t, "main", got)

}
