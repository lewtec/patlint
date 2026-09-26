package ingest_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/ingest"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestWalkSymbols_NonRecursiveDirectory(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "root.go").String(), "package main\n\nfunc Root() {}\n")
	mustWrite(t, lewpath.New(dir, "sub", "sub.go").String(), "package main\n\nfunc Sub() {}\n")

	refs, err := collectRefs(t, dir, "path:./", ingest.ListOptions{IncludeHidden: true})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}

	if !containsRef(refs, "path:./root.go::Root") {
		t.Fatalf("expected root symbol, got %v", refs)
	}
	if containsRef(refs, "path:./sub/sub.go::Sub") {
		t.Fatalf("did not expect nested symbol in non-recursive listing, got %v", refs)
	}
}

func TestWalkSymbols_RecursiveDirectory(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "root.go").String(), "package main\n\nfunc Root() {}\n")
	mustWrite(t, lewpath.New(dir, "sub", "sub.go").String(), "package main\n\nfunc Sub() {}\n")

	refs, err := collectRefs(t, dir, "path:./", ingest.ListOptions{IncludeHidden: true, Recursive: true})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}

	if !containsRef(refs, "path:./root.go::Root") || !containsRef(refs, "path:./sub/sub.go::Sub") {
		t.Fatalf("expected recursive symbols, got %v", refs)
	}
}

func TestWalkSymbols_HiddenFilter(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package main\n\nfunc visible() {}\nfunc _private() {}\nfunc Visible() {}\n")

	refs, err := collectRefs(t, dir, "path:./", ingest.ListOptions{})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if containsRef(refs, "path:./a.go::visible") {
		t.Fatalf("did not expect hidden symbol without IncludeHidden, got %v", refs)
	}
	if containsRef(refs, "path:./a.go::_private") {
		t.Fatalf("did not expect hidden underscore symbol without IncludeHidden, got %v", refs)
	}
	if !containsRef(refs, "path:./a.go::Visible") {
		t.Fatalf("expected exported symbol, got %v", refs)
	}

	refs, err = collectRefs(t, dir, "path:./", ingest.ListOptions{IncludeHidden: true})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if !containsRef(refs, "path:./a.go::visible") {
		t.Fatalf("expected hidden symbol with IncludeHidden, got %v", refs)
	}
	if !containsRef(refs, "path:./a.go::_private") {
		t.Fatalf("expected hidden underscore symbol with IncludeHidden, got %v", refs)
	}
}

func TestWalkSymbols_FileScope(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package main\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "b.go").String(), "package main\n\nfunc B() {}\n")

	refs, err := collectRefs(t, dir, "path:./a.go", ingest.ListOptions{IncludeHidden: true})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if len(refs) != 1 || refs[0] != "path:./a.go::A" {
		t.Fatalf("expected only a.go symbol, got %v", refs)
	}
}

func TestWalkSymbols_StopEarly(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package main\n\nfunc A() {}\nfunc B() {}\n")

	count := 0
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WalkAtoms(t.Context(), dir, "path:./", ingest.ListOptions{IncludeHidden: true}, func(sym ingest.AtomInfo) bool {
		_ = sym
		count++
		return false
	})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected early stop after 1 item, got %d", count)
	}
}

func TestWalkSymbols_GoProviderScope(t *testing.T) {
	refs, err := collectRefs(t, ".", "go:fmt", ingest.ListOptions{})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if !containsSymbol(refs, "Printf") {
		t.Fatalf("expected Printf in go:fmt listing, got %d refs", len(refs))
	}
}

func TestWalkSymbols_UnsupportedProvider(t *testing.T) {
	_, err := collectRefs(t, ".", "node:react", ingest.ListOptions{})
	if err == nil {
		t.Fatal("expected error for unsupported provider listing")
	}
	if !errors.Is(err, ingest.ErrListingNotSupported) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWalkSymbols_GoProviderReferenceShape(t *testing.T) {
	refs, err := collectRefs(t, ".", "go:fmt", ingest.ListOptions{})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("expected symbols")
	}

	for _, r := range refs {
		if !strings.HasPrefix(r, "go:fmt::") {
			t.Fatalf("unexpected provider reference: %q", r)
		}
		if strings.Contains(r, "::Test") || strings.Contains(r, "::Example") {
			t.Fatalf("did not expect go test/example symbols in provider listing: %q", r)
		}
	}
}

func mustWrite(t *testing.T, file, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func collectRefs(t *testing.T, dir, ref string, opts ingest.ListOptions) ([]string, error) {
	t.Helper()
	out := []string{}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WalkAtoms(t.Context(), dir, ref, opts, func(sym ingest.AtomInfo) bool {
		out = append(out, sym.Atom.Reference)
		return true
	})
	return out, err
}

func containsSymbol(refs []string, symbol string) bool {
	for _, r := range refs {
		ref := ingest.ParseReference(strings.TrimSpace(r))
		if ref.Name == symbol {
			return true
		}
	}
	return false
}

func containsRef(refs []string, needle string) bool {
	for _, r := range refs {
		if strings.TrimSpace(r) == needle {
			return true
		}
	}
	return false
}
