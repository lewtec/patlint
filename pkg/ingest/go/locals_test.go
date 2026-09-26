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
	"github.com/stretchr/testify/require"
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
	pf, err := ingestutil.ParseSource(t.Context(), ccgo.Engine{}, src, "x.go", "go")
	require.NoError(t, err)

	defer pf.Close()
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	idx := ingest.LocalBindingsForLanguage(t.Context(), vm, project.NewSession(".").WithEngine(ccgo.Engine{}), pf.Root, src, "go", "x.go")
	require.NotNil(t, idx)

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
	require.False(t, defA.Empty(), "no def for a; sites=%d", len(idx.BySpan))
	require.False(t, useA.Empty(), "no use for a; sites=%d", len(idx.BySpan))

	site, ok := idx.Lookup(useA)
	require.True(t, ok, "use -> def: site=%+v defA=%v", site, defA)
	require.Equal(t, defA, site.DefSpan)
	// O(1) map: every site has an entry.
	require.GreaterOrEqual(t, len(idx.BySpan), 4) // a, b defs + uses

}
