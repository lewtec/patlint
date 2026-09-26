package ingest_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	_ "github.com/lewtec/patlint/pkg/ingest/ecma/js"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	_ "github.com/lewtec/patlint/pkg/ingest/python"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"io/fs"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestRename_MissingEntity(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "main.go").String(), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
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
	_, err = w.Rename(t.Context(), dir, "path:./main.go::doesNotExist", "path:./main.go::renamed")
	if err == nil {
		t.Fatal("expected error for missing entity")
	}
	if !errors.Is(err, ingest.ErrEntityNotFound) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRename_RenamesDefinitionAndCallsite(t *testing.T) {
	dir := t.TempDir()
	// Module path and member names differ so pack as-use on identifiers does not
	// rewrite the module stem when renaming the function.
	if err := os.WriteFile(lewpath.New(dir, "helpers.py").String(), []byte("def helper():\n    pass\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "app.py").String(), []byte("from helpers import helper\n\n\ndef main():\n    helper()\n"), 0644); err != nil {
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
	plan, err := w.Rename(t.Context(), dir, "path:./helpers.py::helper", "path:./helpers.py::renamed")
	if err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	// Pack extract may surface extra file-level use sites; require the three
	// semantic rewrites via post-apply content, not an exact edit count.
	if len(plan.Edits) < 3 {
		t.Fatalf("expected at least 3 edits (definition + import + callsite), got %d", len(plan.Edits))
	}

	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatalf("apply edits failed: %v", err)
	}

	app, err := os.ReadFile(lewpath.New(dir, "app.py").String())
	if err != nil {
		t.Fatal(err)
	}
	helper, err := os.ReadFile(lewpath.New(dir, "helpers.py").String())
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(helper), "def renamed():") {
		t.Fatalf("expected helper definition renamed, got:\n%s", helper)
	}
	if !strings.Contains(string(app), "from helpers import renamed") {
		t.Fatalf("expected imported symbol renamed, got:\n%s", app)
	}
	if !strings.Contains(string(app), "renamed()") {
		t.Fatalf("expected callsite renamed, got:\n%s", app)
	}
}

func TestRename_ShorthandPathReferences(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "helpers.py").String(), []byte("def helper():\n    pass\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "app.py").String(), []byte("from helpers import helper\n\n\ndef main():\n    helper()\n"), 0644); err != nil {
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
	plan, err := w.Rename(t.Context(), dir, "helpers.py::helper", "helpers.py::renamed")
	if err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if len(plan.Edits) < 3 {
		t.Fatalf("expected at least 3 edits (definition + import + callsite), got %d", len(plan.Edits))
	}
}

func TestRename_PythonAliasedImport_RenamesImportedMemberOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "helpers.py").String(), []byte("def helper():\n    pass\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "app.py").String(), []byte("from helpers import helper as h\n\n\ndef main():\n    h()\n"), 0644); err != nil {
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
	plan, err := w.Rename(t.Context(), dir, "path:./helpers.py::helper", "path:./helpers.py::renamed")
	if err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if len(plan.Edits) < 2 {
		t.Fatalf("expected at least 2 edits (definition + imported member), got %d", len(plan.Edits))
	}

	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatalf("apply edits failed: %v", err)
	}

	app, err := os.ReadFile(lewpath.New(dir, "app.py").String())
	if err != nil {
		t.Fatal(err)
	}
	helper, err := os.ReadFile(lewpath.New(dir, "helpers.py").String())
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(helper), "def renamed():") {
		t.Fatalf("expected helper definition renamed, got:\n%s", helper)
	}
	if !strings.Contains(string(app), "from helpers import renamed as h") {
		t.Fatalf("expected imported member renamed, got:\n%s", app)
	}
	if !strings.Contains(string(app), "h()") {
		t.Fatalf("expected aliased callsite to stay as h(), got:\n%s", app)
	}
	if strings.Contains(string(app), "renamed()") {
		t.Fatalf("did not expect aliased callsite renamed, got:\n%s", app)
	}
}

func TestRename_JSAliasedImport_RenamesImportedMemberOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "helper.js").String(), []byte("export function helper() {\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "main.js").String(), []byte("import { helper as h } from \"./helper.js\";\n\nfunction main() {\n  h();\n}\n"), 0644); err != nil {
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
	plan, err := w.Rename(t.Context(), dir, "path:./helper.js::helper", "path:./helper.js::doHelp")
	if err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if len(plan.Edits) < 2 {
		t.Fatalf("expected at least 2 edits (definition + imported member), got %d", len(plan.Edits))
	}

	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatalf("apply edits failed: %v", err)
	}

	main, err := os.ReadFile(lewpath.New(dir, "main.js").String())
	if err != nil {
		t.Fatal(err)
	}
	helper, err := os.ReadFile(lewpath.New(dir, "helper.js").String())
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(helper), "function doHelp()") {
		t.Fatalf("expected helper definition renamed, got:\n%s", helper)
	}
	if !strings.Contains(string(main), "import { doHelp as h }") {
		t.Fatalf("expected imported member renamed, got:\n%s", main)
	}
	if !strings.Contains(string(main), "h();") {
		t.Fatalf("expected aliased callsite to stay as h(), got:\n%s", main)
	}
	if strings.Contains(string(main), "doHelp();") {
		t.Fatalf("did not expect aliased callsite renamed, got:\n%s", main)
	}
}

func TestRename_ModuleAliasMemberCallsite_RenamesMemberAccess(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "helpers.py").String(), []byte("def helper():\n    pass\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "app.py").String(), []byte("import helpers as h\n\n\ndef main():\n    h.helper()\n"), 0644); err != nil {
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
	plan, err := w.Rename(t.Context(), dir, "path:./helpers.py::helper", "path:./helpers.py::renamed")
	if err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if len(plan.Edits) < 2 {
		t.Fatalf("expected at least 2 edits (definition + member callsite), got %d", len(plan.Edits))
	}

	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatalf("apply edits failed: %v", err)
	}

	app, err := os.ReadFile(lewpath.New(dir, "app.py").String())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(app), "h.renamed()") {
		t.Fatalf("expected member access renamed, got:\n%s", app)
	}
}

func TestPackageMove_OnlyGraphConsumersRewritten(t *testing.T) {
	// Moving top-level ./pkg must not rewrite files under nested …/pkg that
	// share only the leaf name and have no import/use edge to the real package.
	dir := t.TempDir()
	if err := os.MkdirAll(lewpath.New(dir, "pkg").String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "pkg", "lib.go").String(), []byte("package pkg\n\nfunc Hello() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "main.go").String(), []byte("package main\n\nimport \"./pkg\"\n\nfunc main() { pkg.Hello() }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Unrelated tree sharing the leaf name "pkg", with path text that a naive
	// segment rewrite would change, but no graph edge to top-level ./pkg.
	nested := lewpath.New(dir, "testdata", "fixture", "input", "pkg").String()
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(nested, "x.go").String(), []byte("package pkg\n\nfunc Local() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	unrelated := lewpath.New(dir, "testdata", "fixture", "input", "other.go").String()
	// Local import to nested pkg only (resolves under testdata/…/input/pkg).
	if err := os.WriteFile(unrelated, []byte("package input\n\nimport nest \"./pkg\"\n\nfunc F() { nest.Local() }\n"), 0644); err != nil {
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
	plan, err := w.Rename(t.Context(), dir, "path:./pkg", "path:./pkga")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	for _, e := range plan.Edits {
		if strings.Contains(e.File, "testdata") {
			t.Fatalf("package move must not edit tree without use of moved package: %+v", e)
		}
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lewpath.New(dir, "pkga", "lib.go").String()); err != nil {
		t.Fatalf("expected pkga/lib.go after move: %v", err)
	}
	// Nested fixture tree unchanged.
	got, err := os.ReadFile(unrelated)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "pkga") {
		t.Fatalf("unrelated file rewritten:\n%s", got)
	}
	if !strings.Contains(string(got), `"./pkg"`) {
		t.Fatalf("local fixture import lost:\n%s", got)
	}
}

func TestPackageMove_ModuleImportPathConsumers(t *testing.T) {
	// Real monorepo shape: go.mod + import module/pkg/sub — must rewrite to module/pkga/sub.
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example.com/mod\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lewpath.New(dir, "pkg", "sub").String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "pkg", "sub", "lib.go").String(), []byte("package sub\n\nfunc Hello() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "main.go").String(), []byte("package main\n\nimport \"example.com/mod/pkg/sub\"\n\nfunc main() { sub.Hello() }\n"), 0644); err != nil {
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
	plan, err := w.Rename(t.Context(), dir, "path:./pkg", "path:./pkga")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if len(plan.DirMoves) != 1 || plan.DirMoves[0].From != "pkg" || plan.DirMoves[0].To != "pkga" {
		t.Fatalf("expected DirMove pkg→pkga, got %+v", plan.DirMoves)
	}
	var sawMain bool
	for _, e := range plan.Edits {
		if e.File == "main.go" {
			sawMain = true
			break
		}
	}
	if !sawMain {
		t.Fatalf("expected consumer rewrite on main.go; edits=%d", len(plan.Edits))
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	main, err := os.ReadFile(lewpath.New(dir, "main.go").String())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(main), "example.com/mod/pkga/sub") {
		t.Fatalf("module import not rewritten:\n%s", main)
	}
	if strings.Contains(string(main), "example.com/mod/pkg/sub") {
		t.Fatalf("old module import still present:\n%s", main)
	}
	if _, err := os.Stat(lewpath.New(dir, "pkga", "sub", "lib.go").String()); err != nil {
		t.Fatalf("package not relocated: %v", err)
	}
}

func TestPackageMove_DestExistsUsesFileLevel(t *testing.T) {
	// Merging into an existing destination cannot DirMove; fall back to file edits.
	dir := t.TempDir()
	if err := os.MkdirAll(lewpath.New(dir, "pkg").String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lewpath.New(dir, "pkga").String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "pkg", "lib.go").String(), []byte("package pkg\n\nfunc Hello() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "pkga", "keep.go").String(), []byte("package pkga\n\nfunc Keep() {}\n"), 0644); err != nil {
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
	plan, err := w.Rename(t.Context(), dir, "path:./pkg", "path:./pkga")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if len(plan.DirMoves) != 0 {
		t.Fatalf("dest exists: expected no DirMove, got %+v", plan.DirMoves)
	}
	if len(plan.Edits) == 0 {
		t.Fatal("expected file-level relocate edits")
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lewpath.New(dir, "pkga", "lib.go").String()); err != nil {
		t.Fatalf("expected merge of lib.go into pkga: %v", err)
	}
	if _, err := os.Stat(lewpath.New(dir, "pkga", "keep.go").String()); err != nil {
		t.Fatalf("pre-existing keep.go lost: %v", err)
	}
}

func TestPackageMove_FileOntoExistingDestReplaces(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(lewpath.New(dir, "out").String(), 0755); err != nil {
		t.Fatal(err)
	}
	src := "const a = 1;\nconst b = 2;\n"
	if err := os.WriteFile(lewpath.New(dir, "src.js").String(), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "out", "dest.js").String(), []byte("const a = 1;\n"), 0644); err != nil {
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
	plan, err := w.Rename(t.Context(), dir, "path:./src.js", "path:./out/dest.js")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(lewpath.New(dir, "out", "dest.js").String())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != src {
		t.Fatalf("dest after file move:\n got %q\nwant %q", got, src)
	}
	if _, err := os.Stat(lewpath.New(dir, "src.js").String()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("source should be gone: %v", err)
	}
}

func TestPackageMove_DirMoveKeepsUntrackedCoLocated(t *testing.T) {
	// Single-pair package move renames the directory so non-ingested files
	// co-located under the package (README, assets, etc.) move with it.
	dir := t.TempDir()
	if err := os.MkdirAll(lewpath.New(dir, "pkg").String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "pkg", "lib.go").String(), []byte("package pkg\n\nfunc Hello() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "pkg", "NOTES.md").String(), []byte("# notes for pkg\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "main.go").String(), []byte("package main\n\nimport \"./pkg\"\n\nfunc main() { pkg.Hello() }\n"), 0644); err != nil {
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
	plan, err := w.Rename(t.Context(), dir, "path:./pkg", "path:./pkga")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if len(plan.DirMoves) != 1 {
		t.Fatalf("expected 1 DirMove, got %+v", plan.DirMoves)
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lewpath.New(dir, "pkga", "NOTES.md").String()); err != nil {
		t.Fatalf("untracked co-located file not moved with package: %v", err)
	}
	if _, err := os.Stat(lewpath.New(dir, "pkg").String()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("old package dir should be gone after DirMove, err=%v", err)
	}
	main, err := os.ReadFile(lewpath.New(dir, "main.go").String())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(main), `"./pkg"`) {
		t.Fatalf("consumer import not rewritten:\n%s", main)
	}
	if !strings.Contains(string(main), `"./pkga"`) {
		t.Fatalf("expected import of ./pkga:\n%s", main)
	}
}

func TestMove_GoCrossFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "doc.go").String(), []byte("package main\n\nfunc helper() {\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "ls.go").String(), []byte("package main\n\nfunc other() {\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "main.go").String(), []byte("package main\n\nfunc main() {\n\thelper()\n}\n"), 0644); err != nil {
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
	plan, err := w.Rename(t.Context(), dir, "doc.go::helper", "ls.go::helper")
	if err != nil {
		t.Fatalf("move failed: %v", err)
	}
	if len(plan.Edits) != 2 {
		t.Fatalf("expected 2 edits (remove+insert), got %d", len(plan.Edits))
	}

	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatalf("apply edits failed: %v", err)
	}

	doc, err := os.ReadFile(lewpath.New(dir, "doc.go").String())
	if err != nil {
		t.Fatal(err)
	}
	ls, err := os.ReadFile(lewpath.New(dir, "ls.go").String())
	if err != nil {
		t.Fatal(err)
	}
	main, err := os.ReadFile(lewpath.New(dir, "main.go").String())
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(doc), "func helper()") {
		t.Fatalf("expected helper removed from doc.go, got:\n%s", doc)
	}
	if !strings.Contains(string(ls), "func helper()") {
		t.Fatalf("expected helper inserted into ls.go, got:\n%s", ls)
	}
	if !strings.Contains(string(main), "helper()") {
		t.Fatalf("expected callsite unchanged, got:\n%s", main)
	}
}

func TestApplyEdits_AppliesDescendingOffsets(t *testing.T) {
	dir := t.TempDir()
	file := lewpath.New(dir, "main.go").String()
	if err := os.WriteFile(file, []byte("alpha beta gamma\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := project.ApplyEdits(t.Context(), dir, []project.Edit{
		{File: "main.go", Span: ingestutil.Span{StartByte: 11, EndByte: 16}, NewText: "G"}, // gamma
		{File: "main.go", Span: ingestutil.Span{StartByte: 0, EndByte: 5}, NewText: "A"},   // alpha
	})
	if err != nil {
		t.Fatalf("apply edits failed: %v", err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "A beta G\n" {
		t.Fatalf("unexpected output: %q", string(got))
	}
}

// New-file creates: first [0,0) is the body; later [0,0) prepend (imports).
// Import text may be longer than a short decl — length must not reorder them.
func TestApplyEdits_NewFileImportPrependsAboveShortDecl(t *testing.T) {
	dir := t.TempDir()
	err := project.ApplyEdits(t.Context(), dir, []project.Edit{
		{File: "dest.py", Span: ingestutil.Span{StartByte: 0, EndByte: 0}, NewText: "ZERO = timedelta(0)\n"},
		{File: "dest.py", Span: ingestutil.Span{StartByte: 0, EndByte: 0}, NewText: "from datetime import timedelta\n\n"},
	})
	if err != nil {
		t.Fatalf("apply edits failed: %v", err)
	}
	got, err := os.ReadFile(lewpath.New(dir, "dest.py").String())
	if err != nil {
		t.Fatal(err)
	}
	want := "from datetime import timedelta\n\nZERO = timedelta(0)\n"
	if string(got) != want {
		t.Fatalf("got:\n%q\nwant:\n%q", string(got), want)
	}
}

func TestApplyEdits_OutOfBounds(t *testing.T) {
	dir := t.TempDir()
	file := lewpath.New(dir, "main.go").String()
	if err := os.WriteFile(file, []byte("abc\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := project.ApplyEdits(t.Context(), dir, []project.Edit{{
		File:    "main.go",
		Span:    ingestutil.Span{StartByte: 0, EndByte: 999},
		NewText: "x",
	}})
	if err == nil {
		t.Fatal("expected out-of-bounds error")
	}
	if !errors.Is(err, ingest.ErrEditOutOfBounds) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyEdits_MissingFile(t *testing.T) {
	dir := t.TempDir()
	err := project.ApplyEdits(t.Context(), dir, []project.Edit{{
		File:    "missing.go",
		Span:    ingestutil.Span{StartByte: 0, EndByte: 1},
		NewText: "x",
	}})
	if err == nil {
		t.Fatal("expected missing file error")
	}
	if !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSymbolLeaf(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"helper", "helper"},
		{"A.Run", "Run"},
		{"*A.Run", "Run"},
		{"*A", "A"},
		{"Outer.Inner.field", "field"},
		// TS/JS string property keys (astro content types: Render.'.md')
		{"Render.'.md'", "'.md'"},
		{`Render.".md"`, `".md"`},
		{`Foo."a.b.c"`, `"a.b.c"`},
		{"Type.'.astro'", "'.astro'"},
		{"Outer.Inner.'.md'", "'.md'"},
	}
	for _, tc := range cases {
		if got := ingest.AtomName(tc.in); got != tc.want {
			t.Errorf("AtomName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
