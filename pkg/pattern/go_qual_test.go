package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestGoQualifiedTypeUse(t *testing.T) {
	src := []byte("package main\nimport \"github.com/spf13/cobra\"\nvar _ *cobra.Command\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "x.go")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "x.go")
	require.NoError(t, err)

	var found bool
	for _, u := range fe.Usages {
		t.Logf("name=%q prefix=%v", u.Name, u.Prefix)
		if u.Name == "Command" && len(u.Prefix) == 1 && u.Prefix[0].Name == "cobra" {
			found = true
		}
	}
	require.True(t, found,
		"want cobra.Command use with name-list prefix cobra")

}
