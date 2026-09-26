package ingest_test

import (
	"os"
	"strings"
	"testing"

	lewio "github.com/lewtec/lewkit/x/io"
	"github.com/stretchr/testify/require"

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
	require.Equal(t, ".other", got,
		"init: got %q", got)

	got = ingest.RewriteImportPathFile("consumer.py", ".helpers", "helpers.py", "utils.py", nil)
	require.Equal(t, ".utils", got,
		"consumer: got %q", got)

}

func TestRewriteImportPathFile_PythonDotted(t *testing.T) {
	got := ingest.RewriteImportPathFile("app.py", "pkga.helpers", "pkga/helpers.py", "pkgb/utils.py", nil)
	require.Equal(t, "pkgb.utils", got,
		"dotted: got %q", got)

	got = ingest.RewriteImportPathFile("pkg/app.py", ".helpers", "pkg/helpers.py", "pkg/utils.py", nil)
	require.Equal(t, ".utils", got,
		"relative: got %q", got)

	got = ingest.RewriteImportPathFile("pkg/a/consumer.py", ".mod", "pkg/a/mod.py", "pkg/b/mod_fuzz.py", nil)
	require.Equal(t, "..b.mod_fuzz", got,
		"up: got %q", got)

}

func TestRewriteImportPathFile_InitRepresentant(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	got := ingest.RewriteImportPathFile(
		"app.py", "pkg", "pkg/__init__.py", "other/__init__.py", vm,
	)
	require.NotEmpty(t, got,
		"want representant rewrite")

}

func TestMatchDirectoryRepresentantFromPrelude(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	for _, rel := range []string{"pkg/__init__.py", "lib/default.nix", "foo/mod.rs", "app/index.js"} {
		require.True(t, vm.MatchDirectoryRepresentant(rel),
			"want representant %q", rel)

	}
	require.False(t, vm.MatchDirectoryRepresentant("src/lib.rs"),
		"lib.rs is crate root, not a representant")

}

func TestRewriteImportPathFile_DirectoryIndex(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	got := ingest.RewriteImportPathFile(
		"lib/server/services/index.ts",
		"./rank",
		"lib/server/services/rank/index.ts",
		"lib/server/services/rank/index_fuzz_8839.ts",
		vm,
	)
	require.Equal(t, "./rank/index_fuzz_8839", got,
		"got %q", got)

}

func TestPackageJSONPathRewrite(t *testing.T) {
	dir := t.TempDir()
	src := []byte("{\n  \"exports\": {\n    \"./astro-jsx\": \"./astro-jsx.d.ts\",\n    \"./client\": \"./client.d.ts\"\n  }\n}\n")
	err := os.WriteFile(lewpath.New(dir, "package.json").String(), src, 0o644)
	require.NoError(t, err)

	edits := ingest.RewritePackageJSONPaths(dir, "astro-jsx.d.ts", "components/astro-jsx.d_fuzz_4.ts")
	got := string(project.ApplyEditsInMemory(src, edits))
	require.True(t, strings.Contains(got, `"./astro-jsx": "./components/astro-jsx.d_fuzz_4.ts"`),
		"got %q", got)
	require.True(t, strings.Contains(got, `"./client": "./client.d.ts"`),
		"rewrote client: %q", got)

}

func TestRebaseRelativeImport(t *testing.T) {
	got := ingest.RebaseRelativeImport("../color.zig", "formats/qoi.zig", "compressions/deflate/qoi.zig")
	require.Equal(t, "../../color.zig", got,
		"got %q", got)

	got = ingest.RebaseRelativeImport("color.zig", "Image.zig", "formats/Image_fuzz_a57d.zig")
	require.Equal(t, "../color.zig", got,
		"got %q", got)

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
	{
		err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example\n\ngo 1.22\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := lewio.Mkdirp(lewpath.New(dir, "oldpkg").String())
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(dir, "oldpkg", "a.go").String(), []byte("package oldpkg\n\nfunc A() {}\n"), 0o644)
		require.NoError(t, err)
	}

	main := "package main\n\nimport \"example/oldpkg\"\n\nfunc main() { oldpkg.A() }\n"
	{
		err := os.WriteFile(lewpath.New(dir, "main.go").String(), []byte(main), 0o644)
		require.NoError(t, err)
	}

	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	res, err := w.Load(t.Context(), ingest.SourceProject(dir), ingest.MaterializeOptions{ExpandImports: true})
	require.NoError(t, err)

	edits := ingest.RewriteMarkedImportPaths("main.go", []byte(main), res, func(a project.Alias) (string, bool) {
		if a.ImportPath == "example/oldpkg" {
			return "example/newpkg", true
		}
		return "", false
	})
	require.Len(t, edits, 1,
		"edits=%d", len(edits))

	got := string(project.ApplyEditsInMemory([]byte(main), edits))
	require.True(t, strings.Contains(got, `"example/newpkg"`),
		"got %q", got)
	require.False(t, strings.Contains(got, `"example/oldpkg"`),
		"old path remains: %q", got)

}

func TestRewriteImportsInFile_MarkedNoDriver(t *testing.T) {
	content := []byte(`(import "./old/pkg")`)
	spec := "./old/pkg"
	start := strings.Index(string(content), spec)
	require.GreaterOrEqual(t, start, 0,
		"spec")

	res := &project.Result{
		Aliases: []project.Alias{{
			Reference:       "path:./main.dsl",
			ImportPath:      spec,
			ImportPathStart: uint32(start),
			ImportPathEnd:   uint32(start + len(spec)),
		}},
	}
	edits := ingest.RewriteImportsInFile(nil, "main.dsl", content, res, "path:./old/pkg", "path:./new/pkg")
	require.Len(t, edits, 1,
		"edits=%d", len(edits))

	got := string(project.ApplyEditsInMemory(content, edits))
	require.True(t, strings.Contains(got, `"./new/pkg"`),
		"got %q", got)

}
