package ingest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/internal/testutil"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	_ "github.com/spf13/cobra"
)

func TestNavigateReference_GoProviderCobraCommand(t *testing.T) {
	root := testutil.ModuleRoot(t)
	ref := ingest.ParseReference("go:github.com/lewtec/lewkit/x/cmd::Flag")
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	nav, got := w.NavigateReference(t.Context(), root, nil, nil, ref)
	if nav == nil || len(nav.Atoms) == 0 {
		t.Fatalf("expected provider hop to load lewkit/x/cmd, atoms=%d got=%s", len(nav.Atoms), got.String())
	}
	if got.Name != "Flag" {
		t.Fatalf("got ref %s want ::Flag", got.String())
	}
	// Must be a path we can open (module cache), not still go:…
	if got.Provider == "go" {
		t.Fatalf("expected path-shaped entity after hop, got %s", got.String())
	}
	def, ok := ingest.DefinitionFromResult(root, nav, got)
	if !ok {
		t.Fatalf("DefinitionFromResult failed for %s", got.String())
	}
	if !strings.Contains(def.Path, "lewkit") || !strings.Contains(def.Path, "cmd") {
		t.Fatalf("def path %q", def.Path)
	}
	if _, err := os.Stat(def.Path); err != nil {
		t.Fatalf("def path missing: %v", err)
	}
}

func TestDefinitionAt_CobraCommandFromUse(t *testing.T) {
	root := testutil.ModuleRoot(t)
	path := lewpath.New(root, "testdata/ingest/go_var_and_selector/scenario/root.go").String()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	idx := strings.Index(string(b), "cobra.Command")
	if idx < 0 {
		t.Fatal("no cobra.Command in fixture")
	}
	off := idx + len("cobra.")
	rel, _ := filepath.Rel(root, path)
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	def, ok := w.DefinitionAt(t.Context(), ingest.DefinitionQuery{
		Root:    root,
		FileRel: filepath.ToSlash(rel),
		Text:    string(b),
		ByteOff: off,
	})
	if !ok {
		t.Fatal("DefinitionAt failed for cobra.Command")
	}
	if !strings.Contains(def.Path, "cobra") || !strings.HasSuffix(def.Path, "command.go") {
		t.Fatalf("def path %q", def.Path)
	}
}
