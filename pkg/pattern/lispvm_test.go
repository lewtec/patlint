package pattern_test

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestLispVM_NewBound(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	lang, ok := vm.HostLanguage("x.go")
	require.False(t, !ok || lang != "go",
		"host go: %q ok=%v", lang, ok)
	require.True(t, vm.RulesForLanguage("go").DirectoryModule,
		"go DirectoryModule from pack family")

}

func TestLispVM_ImportNeedFromRef(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	n, ok := vm.ImportNeedFromRef("go", "@go:fmt::Errorf")
	require.False(t, !ok || n.ImportPath != "fmt",
		"go fmt: %+v ok=%v", n, ok)

	n, ok = vm.ImportNeedFromRef("go", "go:net/http::Get")
	require.False(t, !ok || n.ImportPath != "net/http",
		"net/http: %+v ok=%v", n, ok)
	{

		_, ok := vm.ImportNeedFromRef("go", "path:./pkg/a.go::Foo")
		require.False(t, ok,
			"path ref")
	}
	{

		_, ok := vm.ImportNeedFromRef("go", "go:./local::X")
		require.False(t, ok,
			"relative go path")
	}

	n, ok = vm.ImportNeedFromRef("templ", "go:fmt::Errorf")
	require.False(t, !ok || n.ImportPath != "fmt",
		"templ go-family: %+v ok=%v", n, ok)
	{

		_, ok := vm.ImportNeedFromRef("python", "go:fmt::Errorf")
		require.False(t, ok,
			"python must not import go refs")
	}

	n, ok = vm.ImportNeedFromRef("rust", "rust:std::io::Result")
	require.False(t, !ok || n.ImportPath != "std::io::Result",
		"rust import-ref-name: %+v ok=%v", n, ok)

}

func TestLispVM_Extract_NoProgram(t *testing.T) {
	fe, err := (*pattern.LispVM)(nil).Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "go", nil, nil, "x.go")
	require.False(t, !errors.Is(err, pattern.ErrExtract) || fe != nil,
		"nil vm: fe=%v err=%v", fe, err)

	fe, err = (&pattern.LispVM{}).Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "go", nil, nil, "x.go")
	require.False(t, !errors.Is(err, pattern.ErrExtract) || fe != nil,
		"empty vm: fe=%v err=%v", fe, err)

}

func TestLispVM_ResolveImport(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	ctx := ingest.ImportResolveContext{ImporterPath: "pkg/a.dsl"}
	{
		got := vm.ResolveImport("./lib", ctx)
		require.Equal(t, "path:./pkg/lib", got,
			"rel=%q", got)
	}
	{

		got := vm.ResolveImport("../b", ctx)
		require.Equal(t, "path:./b", got,
			"up=%q", got)
	}
	{

		got := vm.ResolveImport("fmt", ctx)
		require.Empty(t, got,
			"bare dsl spec=%q", got)
	}

	goCtx := ingest.ImportResolveContext{ImporterPath: "pkg/a.go"}
	{
		got := vm.ResolveImport("fmt", goCtx)
		require.Equal(t, "go:fmt", got,
			"go leftover=%q", got)
	}

	got := vm.ResolveImport("./lib", goCtx)
	require.Equal(t, "path:./pkg/lib", got,
		"go rel=%q", got)

}

func TestLispVM_FromString(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("t.rft", `
(under (path "**/*.js")
  (as-language "javascript")
  (as-family "ecma"))
`))
	require.NoError(t, err)

	lang, ok := vm.HostLanguage("app.js")
	require.False(t, !ok || lang != "javascript",
		"host=%q ok=%v", lang, ok)
	{

		lang, ok := vm.HostLanguage("x.go")
		require.False(t, !ok || lang != "go",
			"prelude go after extra src: %q ok=%v", lang, ok)
	}

}

func TestLispVM_NewIgnoresRewriteHeads(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("rw.rft", `
(rewrite (token "x") "y")
`))
	require.NoError(t, err)

	lang, ok := vm.HostLanguage("x.go")
	require.False(t, !ok || lang != "go",
		"prelude go with rewrite src: %q ok=%v", lang, ok)

	extra := vm.ExtraForms()
	require.False(t, len(extra) != 1 || extra[0].Head != "rewrite",
		"extra=%+v", extra)

}

func TestWalker_RequiresBoth(t *testing.T) {
	{
		_, err := walker.NewWalker(t.Context(), nil, nil)
		require.Error(t, err,
			"want error")
	}

	sess := project.NewSession(".").WithEngine(treesitter.Engine{})
	{
		_, err := walker.NewWalker(t.Context(), sess, nil)
		require.Error(t, err,
			"want nil vm error")
	}

	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), sess, vm)
	require.NoError(t, err)
	require.False(t, w.Sess != sess || w.VM != vm,
		"pair")

}

func TestWalker_WalkExtracts_UsesVMPolicy(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.zzz").String()
	{
		err := os.WriteFile(path, []byte("package p\n"), 0o644)
		require.NoError(t, err)
	}

	sess := project.NewSession(dir).WithEngine(treesitter.Engine{})
	src := ingest.SourceHop(dir, path)

	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("zzz.rft", `
(under (path "**/*.zzz")
  (as-language "go")
  (as-family "go"))
`))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), sess, vm)
	require.NoError(t, err)

	var got *project.FileExtract
	{
		err := w.WalkExtracts(t.Context(), src, func(fe *project.FileExtract) bool {
			got = fe
			return true
		})
		require.NoError(t, err)
	}
	require.NotNil(t, got,
		"Walker VM policy must claim .zzz")

}

func TestWalker_WalkAtoms_UsesVMPolicy(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.zzz").String()
	{
		err := os.WriteFile(path, []byte("package p\nfunc Hello() {}\n"), 0o644)
		require.NoError(t, err)
	}

	sess := project.NewSession(dir).WithEngine(treesitter.Engine{})
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("zzz.rft", `
(under (path "**/*.zzz")
  (as-language "go")
  (as-family "go")
  (under (node "function_declaration")
    (as-atom public (take "name" (node "function_declaration")))))
`))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), sess, vm)
	require.NoError(t, err)

	var got []string
	{
		err := w.WalkAtoms(t.Context(), dir, "path:./", ingest.ListOptions{IncludeHidden: true}, func(sym ingest.AtomInfo) bool {
			got = append(got, sym.Reference.Name)
			return true
		})
		require.NoError(t, err)
	}
	require.False(t, len(got) != 1 || got[0] != "Hello",
		"got %v want [Hello]", got)

}

func TestWalker_Grep_UsesVMPolicy(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.zzz").String()
	{
		err := os.WriteFile(path, []byte("package p\n"), 0o644)
		require.NoError(t, err)
	}

	sess := project.NewSession(dir).WithEngine(treesitter.Engine{})
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("zzz.rft", `
(under (path "**/*.zzz")
  (as-language "go")
  (as-family "go"))
`))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), sess, vm)
	require.NoError(t, err)

	hits, err := w.Grep(t.Context(), "go", `(token "package")`, []string{path})
	require.NoError(t, err)
	require.NotEmpty(t, hits,
		"want match on VM-claimed .zzz")

}

func TestWalker_Load_UsesVMPolicy(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.zzz").String()
	{
		err := os.WriteFile(path, []byte("package p\n"), 0o644)
		require.NoError(t, err)
	}

	sess := project.NewSession(dir).WithEngine(treesitter.Engine{})
	src := ingest.SourceHop(dir, path)
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("zzz.rft", `
(under (path "**/*.zzz")
  (as-language "go")
  (as-family "go"))
`))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), sess, vm)
	require.NoError(t, err)

	got, err := w.Load(t.Context(), src, ingest.MaterializeOptions{})
	require.NoError(t, err)
	require.False(t, got == nil || len(got.Files) == 0,
		"Walker.Load must claim .zzz")

}

func TestLispVM_Run_WriteOnly(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	orig := []byte("package p\n\nfunc f(x interface{}) {}\n")
	{
		err := os.WriteFile(path, orig, 0o644)
		require.NoError(t, err)
	}

	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("rw.rft", `
(rewrite
  (under (lang go) (token "interface{}"))
  "any")
`))
	require.NoError(t, err)

	sess := project.NewSession(dir).WithEngine(treesitter.Engine{})
	out, err := vm.Run(t.Context(), sess)
	require.NoError(t, err)
	require.NotNil(t, out,
		"nil session")

	got, err := fs.ReadFile(out.FS, "x.go")
	require.NoError(t, err)
	require.False(t, !strings.Contains(string(got), "interface{}") && !strings.Contains(string(got), "any"),
		"got %q", got)
	require.True(t, strings.Contains(string(got), "any"),
		"want any, got %q", got)

	disk, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(orig), string(disk),
		"Run must not write disk")
	{

		err := ingest.WriteBack(t.Context(), out)
		require.NoError(t, err)
	}

	disk, err = os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(disk), "any"),
		"WriteBack disk=%q", disk)

}
