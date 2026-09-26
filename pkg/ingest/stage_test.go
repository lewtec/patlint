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
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestStageEditsAndValidate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := lewpath.New(dir, "main.go").String()
	src := "package main\n\nfunc Hello() {}\n\nfunc main() { Hello() }\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.Rename(t.Context(), dir, "path:./main.go::Hello", "path:./main.go::Hi")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Edits) == 0 {
		t.Fatal("expected edits")
	}
	ov, err := ingest.StageEdits(t.Context(), dir, nil, plan.Edits)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateStaged(t.Context(), dir, ov); err != nil {
		t.Fatalf("validate: %v", err)
	}
	// disk unchanged
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != src {
		t.Fatalf("disk changed: %q", got)
	}
}
