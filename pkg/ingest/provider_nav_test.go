package ingest_test

import (
	"os"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/internal/testutil"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
	_ "github.com/spf13/cobra"
)

func TestNavigateReference_GoProviderCobraCommand(t *testing.T) {
	root := testutil.ModuleRoot(t)
	ref := ingest.ParseReference("go:github.com/lewtec/lewkit/x/cmd::Flag")
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(root).WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	nav, got := w.NavigateReference(t.Context(), root, nil, nil, ref)
	require.NotNil(t, nav)
	require.NotEmpty(t, nav.Atoms, "expected provider hop to load lewkit/x/cmd, got=%s", got.String())
	require.Equal(t, "Flag", got.Name)
	// Must be a path we can open (module cache), not still go:…
	require.NotEqual(t, "go", got.Provider, "expected path-shaped entity after hop, got %s", got.String())

	def, ok := ingest.DefinitionFromResult(root, nav, got)
	require.True(t, ok, "DefinitionFromResult failed for %s", got.String())
	require.Contains(t, def.Path, "lewkit")
	require.Contains(t, def.Path, "cmd")
	_, err = os.Stat(def.Path)
	require.NoError(t, err)
}
