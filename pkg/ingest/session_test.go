package ingest_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestSession_WalksWithoutCache(t *testing.T) {
	dir := t.TempDir()
	src := "package p\n\nfunc F() {}\n"
	if err := os.WriteFile(lewpath.New(dir, "f.go").String(), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), sess, vm)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := w.WalkAtoms(t.Context(), dir, "path:./", ingest.ListOptions{Recursive: true}, func(ingest.AtomInfo) bool {
		n++
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected atoms")
	}
}
