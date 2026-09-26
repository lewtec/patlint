package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestGoQualifiedTypeUse(t *testing.T) {
	src := []byte("package main\nimport \"github.com/spf13/cobra\"\nvar _ *cobra.Command\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, u := range fe.Usages {
		t.Logf("name=%q prefix=%v", u.Name, u.Prefix)
		if u.Name == "Command" && len(u.Prefix) == 1 && u.Prefix[0].Name == "cobra" {
			found = true
		}
	}
	if !found {
		t.Fatal("want cobra.Command use with name-list prefix cobra")
	}
}
