package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestGoUseScopeInsideMain(t *testing.T) {
	src := []byte("package main\n\nfunc main() {\n\thelper()\n}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "main.go")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "main.go")
	require.NoError(t, err)

	var gotScope string
	for _, u := range fe.Usages {
		if u.Name == "helper" {
			gotScope = u.Scope
			break
		}
	}
	require.Equal(t, "main", gotScope,
		"helper use Scope=%q want main; usages=%+v", gotScope, fe.Usages)

	dir := t.TempDir()
	{
		err := os.WriteFile(lewpath.New(dir, "main.go").String(), src, 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(dir, "helper.go").String(), []byte("package main\n\nfunc helper() {}\n"), 0o644)
		require.NoError(t, err)
	}

	sess := project.NewSession(dir).WithEngine(treesitter.Engine{})
	walk, err := walker.NewWalker(t.Context(), sess, vm)
	require.NoError(t, err)

	res, err := walk.Load(t.Context(), ingest.SourceDir(dir, "", true), ingest.MaterializeOptions{})
	require.NoError(t, err)

	found := false
	for _, u := range res.Uses {
		if u.Target == "path:./helper.go::helper" && u.Reference == "path:./main.go::main" {
			found = true
			break
		}
	}
	require.True(t, found,
		"want use main.go::main -> helper.go::helper; uses=%+v", res.Uses)

}
