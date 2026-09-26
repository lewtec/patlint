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

	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestLispVM_NewBound(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	if lang, ok := vm.HostLanguage("x.go"); !ok || lang != "go" {
		t.Fatalf("host go: %q ok=%v", lang, ok)
	}
	if !vm.RulesForLanguage("go").DirectoryModule {
		t.Fatal("go DirectoryModule from pack family")
	}
}

func TestLispVM_ImportNeedFromRef(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	n, ok := vm.ImportNeedFromRef("go", "@go:fmt::Errorf")
	if !ok || n.ImportPath != "fmt" {
		t.Fatalf("go fmt: %+v ok=%v", n, ok)
	}
	n, ok = vm.ImportNeedFromRef("go", "go:net/http::Get")
	if !ok || n.ImportPath != "net/http" {
		t.Fatalf("net/http: %+v ok=%v", n, ok)
	}
	if _, ok := vm.ImportNeedFromRef("go", "path:./pkg/a.go::Foo"); ok {
		t.Fatal("path ref")
	}
	if _, ok := vm.ImportNeedFromRef("go", "go:./local::X"); ok {
		t.Fatal("relative go path")
	}
	n, ok = vm.ImportNeedFromRef("templ", "go:fmt::Errorf")
	if !ok || n.ImportPath != "fmt" {
		t.Fatalf("templ go-family: %+v ok=%v", n, ok)
	}
	if _, ok := vm.ImportNeedFromRef("python", "go:fmt::Errorf"); ok {
		t.Fatal("python must not import go refs")
	}
	n, ok = vm.ImportNeedFromRef("rust", "rust:std::io::Result")
	if !ok || n.ImportPath != "std::io::Result" {
		t.Fatalf("rust import-ref-name: %+v ok=%v", n, ok)
	}
}

func TestLispVM_Extract_NoProgram(t *testing.T) {
	fe, err := (*pattern.LispVM)(nil).Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "go", nil, nil, "x.go")
	if !errors.Is(err, pattern.ErrExtract) || fe != nil {
		t.Fatalf("nil vm: fe=%v err=%v", fe, err)
	}
	fe, err = (&pattern.LispVM{}).Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "go", nil, nil, "x.go")
	if !errors.Is(err, pattern.ErrExtract) || fe != nil {
		t.Fatalf("empty vm: fe=%v err=%v", fe, err)
	}
}

func TestLispVM_ResolveImport(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	ctx := ingest.ImportResolveContext{ImporterPath: "pkg/a.dsl"}
	if got := vm.ResolveImport("./lib", ctx); got != "path:./pkg/lib" {
		t.Fatalf("rel=%q", got)
	}
	if got := vm.ResolveImport("../b", ctx); got != "path:./b" {
		t.Fatalf("up=%q", got)
	}
	if got := vm.ResolveImport("fmt", ctx); got != "" {
		t.Fatalf("bare dsl spec=%q", got)
	}
	goCtx := ingest.ImportResolveContext{ImporterPath: "pkg/a.go"}
	if got := vm.ResolveImport("fmt", goCtx); got != "go:fmt" {
		t.Fatalf("go leftover=%q", got)
	}
	if got := vm.ResolveImport("./lib", goCtx); got != "path:./pkg/lib" {
		t.Fatalf("go rel=%q", got)
	}
}

func TestLispVM_FromString(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("t.rft", `
(under (path "**/*.js")
  (as-language "javascript")
  (as-family "ecma"))
`))
	if err != nil {
		t.Fatal(err)
	}
	lang, ok := vm.HostLanguage("app.js")
	if !ok || lang != "javascript" {
		t.Fatalf("host=%q ok=%v", lang, ok)
	}
	if lang, ok := vm.HostLanguage("x.go"); !ok || lang != "go" {
		t.Fatalf("prelude go after extra src: %q ok=%v", lang, ok)
	}
}

func TestLispVM_NewIgnoresRewriteHeads(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("rw.rft", `
(rewrite (token "x") "y")
`))
	if err != nil {
		t.Fatal(err)
	}
	if lang, ok := vm.HostLanguage("x.go"); !ok || lang != "go" {
		t.Fatalf("prelude go with rewrite src: %q ok=%v", lang, ok)
	}
	extra := vm.ExtraForms()
	if len(extra) != 1 || extra[0].Head != "rewrite" {
		t.Fatalf("extra=%+v", extra)
	}
}

func TestWalker_RequiresBoth(t *testing.T) {
	if _, err := walker.NewWalker(t.Context(), nil, nil); err == nil {
		t.Fatal("want error")
	}
	sess := project.NewSession(".").WithEngine(ccgo.Engine{})
	if _, err := walker.NewWalker(t.Context(), sess, nil); err == nil {
		t.Fatal("want nil vm error")
	}
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), sess, vm)
	if err != nil {
		t.Fatal(err)
	}
	if w.Sess != sess || w.VM != vm {
		t.Fatal("pair")
	}
}

func TestWalker_WalkExtracts_UsesVMPolicy(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.zzz").String()
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	src := ingest.SourceHop(dir, path)

	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("zzz.rft", `
(under (path "**/*.zzz")
  (as-language "go")
  (as-family "go"))
`))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), sess, vm)
	if err != nil {
		t.Fatal(err)
	}
	var got *project.FileExtract
	if err := w.WalkExtracts(t.Context(), src, func(fe *project.FileExtract) bool {
		got = fe
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("Walker VM policy must claim .zzz")
	}
}

func TestWalker_WalkAtoms_UsesVMPolicy(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.zzz").String()
	if err := os.WriteFile(path, []byte("package p\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("zzz.rft", `
(under (path "**/*.zzz")
  (as-language "go")
  (as-family "go")
  (under (node "function_declaration")
    (as-atom public (take "name" (node "function_declaration")))))
`))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), sess, vm)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := w.WalkAtoms(t.Context(), dir, "path:./", ingest.ListOptions{IncludeHidden: true}, func(sym ingest.AtomInfo) bool {
		got = append(got, sym.Reference.Name)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "Hello" {
		t.Fatalf("got %v want [Hello]", got)
	}
}

func TestWalker_Grep_UsesVMPolicy(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.zzz").String()
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("zzz.rft", `
(under (path "**/*.zzz")
  (as-language "go")
  (as-family "go"))
`))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), sess, vm)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := w.Grep(t.Context(), "go", `(token "package")`, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("want match on VM-claimed .zzz")
	}
}

func TestWalker_Load_UsesVMPolicy(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.zzz").String()
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	src := ingest.SourceHop(dir, path)
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("zzz.rft", `
(under (path "**/*.zzz")
  (as-language "go")
  (as-family "go"))
`))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), sess, vm)
	if err != nil {
		t.Fatal(err)
	}
	got, err := w.Load(t.Context(), src, ingest.MaterializeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Files) == 0 {
		t.Fatal("Walker.Load must claim .zzz")
	}
}

func TestLispVM_Run_WriteOnly(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	orig := []byte("package p\n\nfunc f(x interface{}) {}\n")
	if err := os.WriteFile(path, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("rw.rft", `
(rewrite
  (under (lang go) (token "interface{}"))
  "any")
`))
	if err != nil {
		t.Fatal(err)
	}
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	out, err := vm.Run(t.Context(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("nil session")
	}
	got, err := fs.ReadFile(out.FS, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "interface{}") && !strings.Contains(string(got), "any") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(string(got), "any") {
		t.Fatalf("want any, got %q", got)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(disk) != string(orig) {
		t.Fatal("Run must not write disk")
	}
	if err := ingest.WriteBack(t.Context(), out); err != nil {
		t.Fatal(err)
	}
	disk, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(disk), "any") {
		t.Fatalf("WriteBack disk=%q", disk)
	}
}
