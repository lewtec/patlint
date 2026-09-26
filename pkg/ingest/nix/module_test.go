package nix_test

import (
	"os"
	"path/filepath"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	nix "github.com/lewtec/patlint/pkg/ingest/nix"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/ingest"
	refpkg "github.com/lewtec/patlint/pkg/reference"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestReferenceProvider_ListScopeChildren(t *testing.T) {
	root := t.TempDir()
	nixpkgsRoot := lewpath.New(root, "nixpkgs").String()
	libDir := lewpath.New(nixpkgsRoot, "lib").String()
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(nixpkgsRoot, "default.nix").String(), []byte("import ./lib/default.nix\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(nixpkgsRoot, "release.nix").String(), []byte("{ }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(libDir, "default.nix").String(), []byte("{\n  id = x: x;\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	old := os.Getenv("NIX_PATH")
	t.Cleanup(func() { _ = os.Setenv("NIX_PATH", old) })
	if err := os.Setenv("NIX_PATH", "nixpkgs="+nixpkgsRoot); err != nil {
		t.Fatal(err)
	}

	children, ok, err := (nix.ReferenceProvider{}).ListScopeChildren(t.Context(), ingest.ParseReference("nix:nixpkgs"), "", false)
	if err != nil {
		t.Fatalf("list scope children failed: %v", err)
	}
	if !ok {
		t.Fatal("expected nix provider child listing")
	}

	if !hasScopeChild(children, refpkg.ScopeChild{Ref: ingest.ParseReference("nix:nixpkgs/lib"), Kind: refpkg.ScopeChildDir}) {
		t.Fatalf("expected lib directory child, got %+v", children)
	}
	if !hasScopeChild(children, refpkg.ScopeChild{Ref: ingest.ParseReference("nix:nixpkgs/release.nix"), Kind: refpkg.ScopeChildFile}) {
		t.Fatalf("expected release.nix file child, got %+v", children)
	}
}

func TestWalkSymbols_NixProviderScope(t *testing.T) {
	root := t.TempDir()
	nixpkgsRoot := lewpath.New(root, "nixpkgs").String()
	libDir := lewpath.New(nixpkgsRoot, "lib").String()
	mustWriteFile(t, lewpath.New(libDir, "default.nix").String(), "{\n  id = x: x;\n}\n")

	old := os.Getenv("NIX_PATH")
	t.Cleanup(func() { _ = os.Setenv("NIX_PATH", old) })
	if err := os.Setenv("NIX_PATH", "nixpkgs="+nixpkgsRoot); err != nil {
		t.Fatal(err)
	}

	out := []string{}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WalkAtoms(t.Context(), ".", "nix:nixpkgs/lib", ingest.ListOptions{IncludeHidden: false}, func(sym ingest.AtomInfo) bool {
		out = append(out, sym.Atom.Reference)
		return true
	})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if len(out) != 1 || out[0] != "nix:nixpkgs/lib::id" {
		t.Fatalf("unexpected refs: %v", out)
	}
}

func hasScopeChild(children []refpkg.ScopeChild, want refpkg.ScopeChild) bool {
	for _, child := range children {
		if child.Kind == want.Kind && child.Ref == want.Ref {
			return true
		}
	}
	return false
}

func mustWriteFile(t *testing.T, file, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
