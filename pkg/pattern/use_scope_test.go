package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestGoUseScopeInsideMain(t *testing.T) {
	src := []byte("package main\n\nfunc main() {\n\thelper()\n}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	var gotScope string
	for _, u := range fe.Usages {
		if u.Name == "helper" {
			gotScope = u.Scope
			break
		}
	}
	if gotScope != "main" {
		t.Fatalf("helper use Scope=%q want main; usages=%+v", gotScope, fe.Usages)
	}

	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "main.go").String(), src, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "helper.go").String(), []byte("package main\n\nfunc helper() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	walk, err := walker.NewWalker(t.Context(), sess, vm)
	if err != nil {
		t.Fatal(err)
	}
	res, err := walk.Load(t.Context(), ingest.SourceDir(dir, "", true), ingest.MaterializeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range res.Uses {
		if u.Target == "path:./helper.go::helper" && u.Reference == "path:./main.go::main" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want use main.go::main -> helper.go::helper; uses=%+v", res.Uses)
	}
}
