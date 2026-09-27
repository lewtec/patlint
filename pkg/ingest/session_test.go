package ingest_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestSession_WalksWithoutCache(t *testing.T) {
	dir := t.TempDir()
	src := "package p\n\nfunc F() {}\n"
	{
		err := os.WriteFile(lewpath.New(dir, "f.go").String(), []byte(src), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example\n\ngo 1.22\n"), 0o644)
		require.NoError(t, err)
	}

	sess := project.NewSession(dir).WithEngine(treesitter.Engine{})
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), sess, vm)
	require.NoError(t, err)

	var n int
	{
		err := w.WalkAtoms(t.Context(), dir, "path:./", ingest.ListOptions{Recursive: true}, func(ingest.AtomInfo) bool {
			n++
			return true
		})
		require.NoError(t, err)
	}
	require.NotEqual(t, n, 0,
		"expected atoms")

}
