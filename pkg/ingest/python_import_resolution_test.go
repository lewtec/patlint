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
	_ "github.com/lewtec/patlint/pkg/ingest/python"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestIngest_PythonRelativeImportResolvesToLocalFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(lewpath.New(dir, "pkg").String(), 0755); err != nil {
		t.Fatal(err)
	}

	app := "from .localmod import helper\n\n\ndef main():\n    return helper()\n"
	local := "def helper():\n    return 1\n"
	if err := os.WriteFile(lewpath.New(dir, "pkg", "app.py").String(), []byte(app), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "pkg", "localmod.py").String(), []byte(local), 0644); err != nil {
		t.Fatal(err)
	}

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	result, err := w.Load(t.Context(), ingest.SourceProject(dir), ingest.MaterializeOptions{ExpandImports: true})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	hasAlias := false
	hasRelation := false
	for _, a := range result.Aliases {
		if a.Reference == "path:./pkg/app.py" && a.Target == "path:./pkg/localmod.py::helper" {
			hasAlias = true
		}
	}
	// Pack as-use is file-scoped (no enclosing def on Reference) until rft
	// models scopes; accept file or ::main reference with the right target.
	for _, rel := range result.Uses {
		if rel.Target != "path:./pkg/localmod.py::helper" {
			continue
		}
		if rel.Reference == "path:./pkg/app.py" || rel.Reference == "path:./pkg/app.py::main" {
			hasRelation = true
		}
	}

	if !hasAlias {
		t.Fatalf("expected relative import alias target path:./pkg/localmod.py::helper, got aliases: %+v", result.Aliases)
	}
	if !hasRelation {
		t.Fatalf("expected helper callsite target path:./pkg/localmod.py::helper, got relations: %+v", result.Uses)
	}
}

func TestIngest_PythonAbsoluteDottedImportResolvesToLocalFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(lewpath.New(dir, "pkg").String(), 0755); err != nil {
		t.Fatal(err)
	}

	app := "from pkg.sub import helper\n\n\ndef main():\n    return helper()\n"
	sub := "def helper():\n    return 1\n"
	if err := os.WriteFile(lewpath.New(dir, "app.py").String(), []byte(app), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "pkg", "sub.py").String(), []byte(sub), 0644); err != nil {
		t.Fatal(err)
	}

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	result, err := w.Load(t.Context(), ingest.SourceProject(dir), ingest.MaterializeOptions{ExpandImports: true})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	hasAlias := false
	hasRelation := false
	for _, a := range result.Aliases {
		if a.Reference == "path:./app.py" && a.Target == "path:./pkg/sub.py::helper" {
			hasAlias = true
		}
	}
	for _, rel := range result.Uses {
		if rel.Target != "path:./pkg/sub.py::helper" {
			continue
		}
		if rel.Reference == "path:./app.py" || rel.Reference == "path:./app.py::main" {
			hasRelation = true
		}
	}

	if !hasAlias {
		t.Fatalf("expected absolute dotted import alias target path:./pkg/sub.py::helper, got aliases: %+v", result.Aliases)
	}
	if !hasRelation {
		t.Fatalf("expected helper callsite target path:./pkg/sub.py::helper, got relations: %+v", result.Uses)
	}
}
