package ingest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestCanonicalizeReference_ReexportChain(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := lewpath.New(root, filepath.FromSlash(rel)).String()
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("barrel/index.js", "export * from \"./inner.js\";\n")
	write("barrel/inner.js", "export { real } from \"./impl.js\";\n")
	write("barrel/impl.js", "export function real() {}\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	got := w.CanonicalizeReference(t.Context(), root, ingest.ParseReference("path:./barrel/index.js::real"))
	want := "path:./barrel/impl.js::real"
	if got.String() != want {
		t.Fatalf("got %q want %q", got.String(), want)
	}
}

func TestCanonicalizeInResult_UsesOnlyGraph(t *testing.T) {
	// Minimal hand-built Result — no filesystem.
	result := &project.Result{
		Atoms: []project.Atom{
			{Reference: "path:./impl.js::real"},
		},
		Aliases: []project.Alias{
			{Reference: "path:./barrel.js", Target: "path:./inner.js"},
			{Reference: "path:./inner.js", Target: "path:./impl.js::real"},
		},
	}
	got := ingest.CanonicalizeInResult(result, ingest.ParseReference("path:./barrel.js::real"))
	if got.String() != "path:./impl.js::real" {
		t.Fatalf("got %q", got.String())
	}
}

func TestCanonicalizePath_PythonPackageDir(t *testing.T) {
	root := t.TempDir()
	pkg := lewpath.New(root, "pkg").String()
	if err := os.MkdirAll(pkg, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkg, "__init__.py").String(), []byte("x = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	got := ingest.CanonicalizePathReference(vm, root, ingest.ParseReference("path:./pkg"))
	if got.String() != "path:./pkg/__init__.py" {
		t.Fatalf("got %q", got.String())
	}
}

func TestCanonicalizePath_RustMod(t *testing.T) {
	root := t.TempDir()
	mod := lewpath.New(root, "foo").String()
	if err := os.MkdirAll(mod, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(mod, "mod.rs").String(), []byte("pub fn f() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "lib.rs").String(), []byte("mod foo;\n"), 0644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	got := ingest.CanonicalizePathReference(vm, root, ingest.ParseReference("path:./foo"))
	if got.String() != "path:./foo/mod.rs" {
		t.Fatalf("got %q", got.String())
	}
	crate := ingest.CanonicalizePathReference(vm, root, ingest.ParseReference("path:./"))
	if crate.Path == "lib.rs" || strings.HasSuffix(crate.Path, "/lib.rs") {
		t.Fatalf("lib.rs is crate root, not a dir representant: %q", crate)
	}
}

func TestCanonicalizePath_NixDefault(t *testing.T) {
	root := t.TempDir()
	lib := lewpath.New(root, "lib").String()
	if err := os.MkdirAll(lib, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(lib, "default.nix").String(), []byte("{ }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	got := ingest.CanonicalizePathReference(vm, root, ingest.ParseReference("path:./lib"))
	if got.String() != "path:./lib/default.nix" {
		t.Fatalf("got %q", got.String())
	}
}

func TestCanonicalizeReference_DefaultExportSoleEntity(t *testing.T) {
	root := t.TempDir()
	p := lewpath.New(root, "mod.js").String()
	if err := os.WriteFile(p, []byte("export default function Thing() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	got := w.CanonicalizeReference(t.Context(), root, ingest.ParseReference("path:./mod.js"))
	if got.String() != "path:./mod.js::Thing" {
		t.Fatalf("got %q want path:./mod.js::Thing", got.String())
	}
}

func TestCanonicalizeReference_ExportAsDefaultAmongMany(t *testing.T) {
	// Compiled ESM: export { createIntegration as default } with other helpers in-file.
	root := t.TempDir()
	p := lewpath.New(root, "mod.js").String()
	body := "function helper() {}\nfunction createIntegration() {}\nexport {\n  createIntegration as default\n};\n"
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	got := w.CanonicalizeReference(t.Context(), root, ingest.ParseReference("path:./mod.js"))
	if got.String() != "path:./mod.js::createIntegration" {
		t.Fatalf("got %q want path:./mod.js::createIntegration", got.String())
	}
}

func TestCanonicalizeReference_PesquisarrParaglide(t *testing.T) {
	root := "/home/lucasew/WORKSPACE/OPENSOURCE-own/pesquisarr"
	if _, err := os.Stat(root); err != nil {
		t.Skip("pesquisarr missing")
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	got := w.CanonicalizeReference(t.Context(), root, ingest.ParseReference(
		"path:./node_modules/@inlang/paraglide-js/dist/index.js::paraglideVitePlugin",
	))
	if got.String() != "path:./node_modules/@inlang/paraglide-js/dist/bundler-plugins/vite.js::paraglideVitePlugin" {
		t.Fatalf("got %q", got.String())
	}
}

func TestCanonicalizePathReference_StillDirectoryOnly(t *testing.T) {
	root := t.TempDir()
	p := lewpath.New(root, "mod.js").String()
	if err := os.WriteFile(p, []byte("export default function Thing() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := ingest.CanonicalizePathReference(nil, root, ingest.ParseReference("path:./mod.js"))
	if got.Name != "" {
		t.Fatalf("path-only canonicalize should not set symbol, got %q", got.String())
	}
}

func TestCanonicalizeReference_DefaultAsNamedReexport(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(lewpath.New(root, "barrel.js").String(), []byte(
		"export { default as Search } from './search.js';\n",
	), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "search.js").String(), []byte(
		"export default function Search() {}\n",
	), 0644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	got := w.CanonicalizeReference(t.Context(), root, ingest.ParseReference("path:./barrel.js::Search"))
	if got.String() != "path:./search.js::Search" {
		t.Fatalf("got %q want path:./search.js::Search", got.String())
	}
}
