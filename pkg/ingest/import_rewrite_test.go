package ingest_test

import (
	"os"
	"strings"
	"testing"

	lewio "github.com/lewtec/lewkit/x/io"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestRewriteImportPathFile_Relative(t *testing.T) {
	cases := []struct{ importer, spec, src, dst, want string }{
		{"main.js", "./lib/helpers.js", "lib/helpers.js", "utils/format.js", "./utils/format.js"},
		{"formats.zig", "formats/farbfeld.zig", "formats/farbfeld.zig", "farbfeld.zig", "farbfeld.zig"},
		{"compressions/deflate/BlockWriter.zig", "huffman_encoder.zig", "compressions/deflate/huffman_encoder.zig", "formats/jpeg/huffman_encoder_fuzz_af7a.zig", "../../formats/jpeg/huffman_encoder_fuzz_af7a.zig"},
		{"formats.zig", "formats/qoi.zig", "formats/qoi.zig", "compressions/deflate/qoi.zig", "compressions/deflate/qoi.zig"},
	}
	for _, tc := range cases {
		got := ingest.RewriteImportPathFile(tc.importer, tc.spec, tc.src, tc.dst, nil)
		if got != tc.want {
			t.Errorf("%s %q → %q: got %q want %q", tc.importer, tc.spec, tc.dst, got, tc.want)
		}
	}
}

func TestRewriteImportPathFile_PythonRelativeRoot(t *testing.T) {
	got := ingest.RewriteImportPathFile("__init__.py", ".mod", "mod.py", "other.py", nil)
	if got != ".other" {
		t.Fatalf("init: got %q", got)
	}
	got = ingest.RewriteImportPathFile("consumer.py", ".helpers", "helpers.py", "utils.py", nil)
	if got != ".utils" {
		t.Fatalf("consumer: got %q", got)
	}
}

func TestRewriteImportPathFile_PythonDotted(t *testing.T) {
	got := ingest.RewriteImportPathFile("app.py", "pkga.helpers", "pkga/helpers.py", "pkgb/utils.py", nil)
	if got != "pkgb.utils" {
		t.Fatalf("dotted: got %q", got)
	}
	got = ingest.RewriteImportPathFile("pkg/app.py", ".helpers", "pkg/helpers.py", "pkg/utils.py", nil)
	if got != ".utils" {
		t.Fatalf("relative: got %q", got)
	}
	got = ingest.RewriteImportPathFile("pkg/a/consumer.py", ".mod", "pkg/a/mod.py", "pkg/b/mod_fuzz.py", nil)
	if got != "..b.mod_fuzz" {
		t.Fatalf("up: got %q", got)
	}
}

func TestRewriteImportPathFile_InitRepresentant(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	got := ingest.RewriteImportPathFile(
		"app.py", "pkg", "pkg/__init__.py", "other/__init__.py", vm,
	)
	if got == "" {
		t.Fatal("want representant rewrite")
	}
}

func TestMatchDirectoryRepresentantFromPrelude(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"pkg/__init__.py", "lib/default.nix", "foo/mod.rs", "app/index.js"} {
		if !vm.MatchDirectoryRepresentant(rel) {
			t.Fatalf("want representant %q", rel)
		}
	}
	if vm.MatchDirectoryRepresentant("src/lib.rs") {
		t.Fatal("lib.rs is crate root, not a representant")
	}
}

func TestRewriteImportPathFile_DirectoryIndex(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	got := ingest.RewriteImportPathFile(
		"lib/server/services/index.ts",
		"./rank",
		"lib/server/services/rank/index.ts",
		"lib/server/services/rank/index_fuzz_8839.ts",
		vm,
	)
	if got != "./rank/index_fuzz_8839" {
		t.Fatalf("got %q", got)
	}
}

func TestPackageJSONPathRewrite(t *testing.T) {
	dir := t.TempDir()
	src := []byte("{\n  \"exports\": {\n    \"./astro-jsx\": \"./astro-jsx.d.ts\",\n    \"./client\": \"./client.d.ts\"\n  }\n}\n")
	if err := os.WriteFile(lewpath.New(dir, "package.json").String(), src, 0o644); err != nil {
		t.Fatal(err)
	}
	edits := ingest.RewritePackageJSONPaths(dir, "astro-jsx.d.ts", "components/astro-jsx.d_fuzz_4.ts")
	got := string(project.ApplyEditsInMemory(src, edits))
	if !strings.Contains(got, `"./astro-jsx": "./components/astro-jsx.d_fuzz_4.ts"`) {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, `"./client": "./client.d.ts"`) {
		t.Fatalf("rewrote client: %q", got)
	}
}

func TestRebaseRelativeImport(t *testing.T) {
	got := ingest.RebaseRelativeImport("../color.zig", "formats/qoi.zig", "compressions/deflate/qoi.zig")
	if got != "../../color.zig" {
		t.Fatalf("got %q", got)
	}
	got = ingest.RebaseRelativeImport("color.zig", "Image.zig", "formats/Image_fuzz_a57d.zig")
	if got != "../color.zig" {
		t.Fatalf("got %q", got)
	}
}

func TestRewriteImportPathDir(t *testing.T) {
	cases := []struct{ spec, old, neu, want string }{
		{"example/oldpkg", "oldpkg", "newpkg", "example/newpkg"},
		{"pkg.oldpkg", "pkg/oldpkg", "pkg/newpkg", "pkg.newpkg"},
		{"crate::oldpkg::foo", "oldpkg/foo", "newpkg/foo", "crate::newpkg::foo"},
		{"example/keep", "oldpkg", "newpkg", ""},
	}
	for _, tc := range cases {
		got := ingest.RewriteImportPathDir(tc.spec, tc.old, tc.neu)
		if got != tc.want {
			t.Errorf("%q %q→%q: got %q want %q", tc.spec, tc.old, tc.neu, got, tc.want)
		}
	}
}

func TestRewriteMarkedImportPaths_GoPackage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := lewio.Mkdirp(lewpath.New(dir, "oldpkg").String()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "oldpkg", "a.go").String(), []byte("package oldpkg\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	main := "package main\n\nimport \"example/oldpkg\"\n\nfunc main() { oldpkg.A() }\n"
	if err := os.WriteFile(lewpath.New(dir, "main.go").String(), []byte(main), 0o644); err != nil {
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
	res, err := w.Load(t.Context(), ingest.SourceProject(dir), ingest.MaterializeOptions{ExpandImports: true})
	if err != nil {
		t.Fatal(err)
	}
	edits := ingest.RewriteMarkedImportPaths("main.go", []byte(main), res, func(a project.Alias) (string, bool) {
		if a.ImportPath == "example/oldpkg" {
			return "example/newpkg", true
		}
		return "", false
	})
	if len(edits) != 1 {
		t.Fatalf("edits=%d", len(edits))
	}
	got := string(project.ApplyEditsInMemory([]byte(main), edits))
	if !strings.Contains(got, `"example/newpkg"`) {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, `"example/oldpkg"`) {
		t.Fatalf("old path remains: %q", got)
	}
}

func TestRewriteImportsInFile_MarkedNoDriver(t *testing.T) {
	content := []byte(`(import "./old/pkg")`)
	spec := "./old/pkg"
	start := strings.Index(string(content), spec)
	if start < 0 {
		t.Fatal("spec")
	}
	res := &project.Result{
		Aliases: []project.Alias{{
			Reference:       "path:./main.dsl",
			ImportPath:      spec,
			ImportPathStart: uint32(start),
			ImportPathEnd:   uint32(start + len(spec)),
		}},
	}
	edits := ingest.RewriteImportsInFile(nil, "main.dsl", content, res, "path:./old/pkg", "path:./new/pkg")
	if len(edits) != 1 {
		t.Fatalf("edits=%d", len(edits))
	}
	got := string(project.ApplyEditsInMemory(content, edits))
	if !strings.Contains(got, `"./new/pkg"`) {
		t.Fatalf("got %q", got)
	}
}
