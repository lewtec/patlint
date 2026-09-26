package pattern_test

import (
	"testing"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/store"
	"github.com/stretchr/testify/require"
)

func TestIngestClauses_AsAtom(t *testing.T) {
	prog, err := pattern.LoadExtractPack("ok.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-scope (this))
    (as-atom public (take "name" (node "function_declaration")))))
`)
	require.NoError(t, err)

	cl := prog.IngestClauses("x.go")
	require.Len(t, cl, 2)

	var sawAtom, sawScope bool
	for _, c := range cl {
		if c.Head.Rel == store.RelationAtom {
			sawAtom = true
			require.Len(t, c.Body, 1)
			require.Equal(t, "$as", c.Body[0].Rel)
		}
		if c.Head.Rel == store.RelationScope {
			sawScope = true
		}
	}
	require.True(t, sawAtom, "want atom, got %+v", cl)
	require.True(t, sawScope, "want scope, got %+v", cl)
	require.Empty(t, prog.IngestClauses("x.js"), "js path should skip go actions")
}
