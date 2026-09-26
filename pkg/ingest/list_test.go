package ingest_test

import (
	"encoding/json"
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

	_ "github.com/lewtec/patlint/pkg/ingest/ecma/js"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/java"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/kotlin"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/scala"
	_ "github.com/lewtec/patlint/pkg/ingest/nix"
	_ "github.com/lewtec/patlint/pkg/ingest/python"
	_ "github.com/lewtec/patlint/pkg/ingest/rust"
	_ "github.com/lewtec/patlint/pkg/ingest/zig"
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
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
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

func TestWalkSymbols_ReferenceFixtures(t *testing.T) {
	fixtureRoot := lewpath.New("..", "..", "testdata", "ingest").String()
	input, err := os.ReadFile(lewpath.New(fixtureRoot, "list_reference_cases.json").String())
	if err != nil {
		t.Fatalf("reading list_reference_cases.json: %v", err)
	}

	var cases []struct {
		Name      string `json:"name"`
		Fixture   string `json:"fixture"`
		Reference string `json:"reference"`
		Options   struct {
			IncludeHidden bool `json:"include_hidden"`
			Recursive     bool `json:"recursive"`
		} `json:"options"`
		ExpectedRefs   []string `json:"expected_refs"`
		UnexpectedRefs []string `json:"unexpected_refs"`
	}
	if err := json.Unmarshal(input, &cases); err != nil {
		t.Fatalf("parsing list_reference_cases.json: %v", err)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			dir := lewpath.New(fixtureRoot, tc.Fixture).String()
			refs, err := collectRefs(t, dir, tc.Reference, ingest.ListOptions{
				IncludeHidden: tc.Options.IncludeHidden,
				Recursive:     tc.Options.Recursive,
			})
			if err != nil {
				t.Fatalf("walk symbols: %v", err)
			}

			for _, expected := range tc.ExpectedRefs {
				if !containsRef(refs, expected) {
					t.Fatalf("expected symbol %q not found, got %v", expected, refs)
				}
			}

			for _, unexpected := range tc.UnexpectedRefs {
				if containsRef(refs, unexpected) {
					t.Fatalf("unexpected symbol %q found, got %v", unexpected, refs)
				}
			}
		})
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
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WalkAtoms(t.Context(), dir, ref, opts, func(sym ingest.AtomInfo) bool {
		out = append(out, sym.Atom.Reference)
		return true
	})
	return out, err
}

func containsRef(refs []string, needle string) bool {
	for _, r := range refs {
		if strings.TrimSpace(r) == needle {
			return true
		}
	}
	return false
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

func TestWalkSymbols_JavaPublicFilter(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "Types.java").String(), `package demo;

public class Visible {
    public void shown() {}
    void hidden() {}
}

class Internal {
    public void alsoHiddenType() {}
}
`)

	refs, err := collectRefs(t, dir, "path:./", ingest.ListOptions{})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if !containsRef(refs, "path:./Types.java::Visible") {
		t.Fatalf("expected public type, got %v", refs)
	}
	if !containsRef(refs, "path:./Types.java::Visible.shown") {
		t.Fatalf("expected public method, got %v", refs)
	}
	if containsRef(refs, "path:./Types.java::Visible.hidden") {
		t.Fatalf("did not expect package-private method, got %v", refs)
	}
	if containsRef(refs, "path:./Types.java::Internal") {
		t.Fatalf("did not expect package-private type, got %v", refs)
	}

	refs, err = collectRefs(t, dir, "path:./", ingest.ListOptions{IncludeHidden: true})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if !containsRef(refs, "path:./Types.java::Visible.hidden") || !containsRef(refs, "path:./Types.java::Internal") {
		t.Fatalf("expected hidden symbols with IncludeHidden, got %v", refs)
	}
}

func TestWalkSymbols_ZigPubFilter(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "lib.zig").String(), `
pub fn shown() void {}
fn hidden() void {}
pub const Shown = 1;
const hidden_const = 2;
`)
	refs, err := collectRefs(t, dir, "path:./", ingest.ListOptions{})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if !containsSymbol(refs, "shown") {
		t.Fatalf("expected pub fn, got %v", refs)
	}
	if !containsSymbol(refs, "Shown") {
		t.Fatalf("expected pub const, got %v", refs)
	}
	if containsSymbol(refs, "hidden") {
		t.Fatalf("did not expect private fn, got %v", refs)
	}
	if containsSymbol(refs, "hidden_const") {
		t.Fatalf("did not expect private const, got %v", refs)
	}
	refs, err = collectRefs(t, dir, "path:./", ingest.ListOptions{IncludeHidden: true})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if !containsSymbol(refs, "hidden") || !containsSymbol(refs, "hidden_const") {
		t.Fatalf("expected hidden zig atoms with IncludeHidden, got %v", refs)
	}
}

func TestWalkSymbols_RustPubFilter(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "lib.rs").String(), `
pub fn shown() {}
fn hidden() {}
pub struct Shown {}
struct Hidden {}
`)
	refs, err := collectRefs(t, dir, "path:./", ingest.ListOptions{})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if !containsSymbol(refs, "shown") {
		t.Fatalf("expected pub fn, got %v", refs)
	}
	if !containsSymbol(refs, "Shown") {
		t.Fatalf("expected pub struct, got %v", refs)
	}
	if containsSymbol(refs, "hidden") {
		t.Fatalf("did not expect private fn, got %v", refs)
	}
	if containsSymbol(refs, "Hidden") {
		t.Fatalf("did not expect private struct, got %v", refs)
	}
	refs, err = collectRefs(t, dir, "path:./", ingest.ListOptions{IncludeHidden: true})
	if err != nil {
		t.Fatalf("walk symbols: %v", err)
	}
	if !containsSymbol(refs, "hidden") || !containsSymbol(refs, "Hidden") {
		t.Fatalf("expected hidden rust atoms with IncludeHidden, got %v", refs)
	}
}
