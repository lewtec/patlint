package nix_test

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/nix"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/walker"
)

func TestNixAbstractHoles(t *testing.T) {
	src := `
let
  maxAb = a: b: if a > b then a else b;
  maxXy = x: y: if x > y then x else y;
  maxSet = { a, b }: if a > b then a else b;
in { inherit maxAb maxXy maxSet; }
`
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, []byte(src), "x.nix", "nix")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	units, err := w.AbstractFile(t.Context(), pf.Root, pf.Source, "x.nix", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 2 {
		t.Fatalf("units=%d %#v", len(units), units)
	}
	var holeful int
	for _, u := range units {
		got := ingest.FormatTerms(u.Terms)
		if strings.Contains(got, "%r1") {
			holeful++
		}
		// param names should not remain as surface for bound uses
		if strings.Contains(got, "if a >") || strings.Contains(got, "if x >") {
			t.Fatalf("surface names remain in %s: %s", u.Name, got)
		}
	}
	if holeful < 2 {
		t.Fatalf("expected holeful units, got %d / %d", holeful, len(units))
	}
}

func parseNix(t *testing.T, src string) (*sitter.Node, []byte, func()) {
	t.Helper()
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, []byte(src), "x.nix", "nix")
	if err != nil {
		t.Fatal(err)
	}
	return pf.Root, pf.Source, pf.Close
}

func TestNixInheritFromLibFormal(t *testing.T) {
	// nixpkgs-style module header: { lib }: let inherit (lib.strings) …;
	src := `{ lib }:

let
  inherit (lib.strings)
    concatStringsSep
    ;
  inherit (lib.lists)
    filter
    ;
in
  concatStringsSep
`
	root, source, cleanup := parseNix(t, src)
	defer cleanup()
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	idx := ingest.LocalBindingsForLanguage(t.Context(), vm, project.NewSession(".").WithEngine(ccgo.Engine{}), root, source, "nix", "mod.nix")
	if idx == nil {
		t.Fatal("nil index")
	}

	// Find formal def span for "lib"
	var libDef ingestutil.Span
	var libUses int
	for sp, site := range idx.BySpan {
		if site.Name != "lib" {
			continue
		}
		if site.IsDef {
			libDef = site.DefSpan
		} else {
			libUses++
			if site.DefSpan.Empty() {
				t.Fatalf("use at %v missing DefSpan", sp)
			}
		}
	}
	if libDef.Empty() {
		t.Fatal("lib formal not defined")
	}
	// Three inherit (lib.… ) uses
	if libUses < 2 {
		t.Fatalf("lib uses=%d want >=2 (inherit from)", libUses)
	}

	// Inherited names defined in let
	var sawConcat bool
	for _, site := range idx.BySpan {
		if site.IsDef && site.Name == "concatStringsSep" {
			sawConcat = true
		}
	}
	if !sawConcat {
		t.Fatal("concatStringsSep not defined from inherit")
	}
}

func TestNixRecursiveLetSelf(t *testing.T) {
	// nixpkgs lib/default.nix: self = rattrs self // { … };
	src := `
let
  rattrs = x: x;
  self = rattrs self // {
    a = 1;
  };
in self
`
	root, source, cleanup := parseNix(t, src)
	defer cleanup()
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	idx := ingest.LocalBindingsForLanguage(t.Context(), vm, project.NewSession(".").WithEngine(ccgo.Engine{}), root, source, "nix", "lib.nix")
	if idx == nil {
		t.Fatal("nil index")
	}
	var selfUses int
	for sp, site := range idx.BySpan {
		if site.Name != "self" || site.IsDef {
			continue
		}
		selfUses++
		_ = sp
	}
	// at least: RHS `rattrs self` and final `in self`
	if selfUses < 2 {
		t.Fatalf("self uses=%d want >=2 (recursive let)", selfUses)
	}
}

func TestNixRelativePathTypeAsImport(t *testing.T) {
	// Any Nix path_expression (./…, ../…, .) is a reference — not only `import PATH`.
	// Covers callPackage ./pkg {}, pkgs.callPackage ./x.nix {}, src = ./.
	root := t.TempDir()
	pkg := lewpath.New(root, "pkg").String()
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.nix", "b.nix"} {
		if err := os.WriteFile(lewpath.New(pkg, name).String(), []byte("{ }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := `{
  a = callPackage ./a.nix { };
  b = pkgs.callPackage ./b.nix { };
  src = ./.;
}
`
	main := lewpath.New(pkg, "default.nix").String()
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	result, err := w.Load(t.Context(), ingest.SourceSeed(root, main), ingest.MaterializeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// src = ./. → pkg/ has default.nix → path:./pkg/default.nix (Nix dir import).
	want := map[string]bool{
		"path:./pkg/a.nix":       false,
		"path:./pkg/b.nix":       false,
		"path:./pkg/default.nix": false,
	}
	for _, a := range result.Aliases {
		t.Logf("alias target=%q", a.Target)
		if _, ok := want[a.Target]; ok {
			want[a.Target] = true
		}
	}
	for target, saw := range want {
		if !saw {
			t.Errorf("missing path ref %s; aliases=%+v", target, result.Aliases)
		}
	}
}

func TestNixSelectBasePathRef(t *testing.T) {
	// (callPackage ./foo {}).bar must still see ./foo as a path ref.
	root := t.TempDir()
	pkg := lewpath.New(root, "pkg").String()
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkg, "foo.nix").String(), []byte("{ bar = 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `{
  x = (callPackage ./foo.nix { }).bar;
}
`
	main := lewpath.New(pkg, "default.nix").String()
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	result, err := w.Load(t.Context(), ingest.SourceSeed(root, main), ingest.MaterializeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "path:./pkg/foo.nix"
	for _, a := range result.Aliases {
		if a.Target == want {
			return
		}
	}
	t.Fatalf("want %s under select/callPackage; aliases=%+v", want, result.Aliases)
}

func TestNixDirectoryPathExpandsDefaultNix(t *testing.T) {
	// callPackage ../applications/foo { } → path:./applications/foo/default.nix
	root := t.TempDir()
	top := lewpath.New(root, "pkgs", "top-level").String()
	app := lewpath.New(root, "pkgs", "applications", "foo").String()
	if err := os.MkdirAll(top, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(app, "default.nix").String(), []byte("{ }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `{
  foo = callPackage ../applications/foo { };
}
`
	main := lewpath.New(top, "all-packages.nix").String()
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	result, err := w.Load(t.Context(), ingest.SourceSeed(root, main), ingest.MaterializeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "path:./pkgs/applications/foo/default.nix"
	for _, a := range result.Aliases {
		t.Logf("alias target=%q", a.Target)
		if a.Target == want {
			return
		}
	}
	t.Fatalf("want %s from directory path; aliases=%+v", want, result.Aliases)
}

func TestNixImportPathAsPathRef(t *testing.T) {
	root := t.TempDir()
	// Nested importer: ./ and ./. are relative to pkg/, not project root.
	pkg := lewpath.New(root, "pkg").String()
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkg, "foo.nix").String(), []byte("{ x = 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `let
  other = import ./foo.nix;
  dir = import ./.;
in other
`
	main := lewpath.New(pkg, "main.nix").String()
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	result, err := w.Load(t.Context(), ingest.SourceSeed(root, main), ingest.MaterializeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var sawFoo, sawDir bool
	for _, a := range result.Aliases {
		t.Logf("alias span=[%d,%d) target=%q text=%q", a.StartByte, a.EndByte, a.Target, src[a.StartByte:a.EndByte])
		switch a.Target {
		case "path:./pkg/foo.nix":
			sawFoo = true
		case "path:./pkg":
			// import ./. → directory of main.nix
			sawDir = true
		}
	}
	if !sawFoo {
		t.Fatalf("want path:./pkg/foo.nix (relative to importer), aliases=%+v", result.Aliases)
	}
	if !sawDir {
		t.Fatalf("want path:./pkg for import ./. (importer dir), aliases=%+v", result.Aliases)
	}

	pf, err := ingestutil.ParseSource(ccgo.Engine{}, []byte(src), "pkg/main.nix", "nix")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	units, err := w.AbstractFileResult(t.Context(), pf.Root, pf.Source, "pkg/main.nix", false, result)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, u := range units {
		joined += ingest.FormatTerms(u.Terms) + " "
	}
	t.Logf("abstract: %s", joined)
	if !strings.Contains(joined, `@path:./pkg/foo.nix`) {
		t.Fatalf("want @path:./pkg/foo.nix, got %s", joined)
	}
	if !strings.Contains(joined, `@path:./pkg`) {
		t.Fatalf("want @path:./pkg for ./., got %s", joined)
	}
	if strings.Contains(joined, "import ./foo.nix") {
		t.Fatalf("path should be @path ref, not surface: %s", joined)
	}
}
