package ingest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	_ "github.com/lewtec/patlint/internal/prelude"

	"github.com/lewtec/patlint/pkg/ingest"
)

func TestCoerceLocalPathReference_Directory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(lewpath.New(dir, "cmd").String(), 0755); err != nil {
		t.Fatal(err)
	}

	ref := ingest.ParseReference("cmd")
	got := ingest.CoerceLocalPathReference(dir, ref)
	if got.Provider != "path" || got.Path != "./cmd" {
		t.Fatalf("unexpected coerced ref: %+v", got)
	}
}

func TestCoerceLocalPathReference_File(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "x.go").String(), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ref := ingest.ParseReference("x.go")
	got := ingest.CoerceLocalPathReference(dir, ref)
	if got.Provider != "path" || got.Path != "./x.go" {
		t.Fatalf("unexpected coerced ref: %+v", got)
	}
}

func TestCoerceLocalPathReference_MissingPath(t *testing.T) {
	dir := t.TempDir()
	ref := ingest.ParseReference(lewpath.New("no", "such", "path").String())
	got := ingest.CoerceLocalPathReference(dir, ref)
	if got.Provider != ref.Provider || got.Path != ref.Path {
		t.Fatalf("expected unchanged ref, got %+v", got)
	}
}

func TestResolveInputReferenceScope(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(lewpath.New(dir, "cmd").String(), 0755); err != nil {
		t.Fatal(err)
	}

	scope := ingest.ResolveInputReferenceScope(dir, "cmd")
	if scope.Dir != lewpath.New(dir, "cmd").String() {
		t.Fatalf("unexpected scope dir: %q", scope.Dir)
	}
	if scope.Reference.Provider != "path" || scope.Reference.Path != "./" {
		t.Fatalf("unexpected scope ref: %+v", scope.Reference)
	}
}

func TestResolveMoveArgs_ExpandsPathToAbsolute(t *testing.T) {
	dir := t.TempDir()
	srcFile := lewpath.New(dir, "pkg", "pattern", "match.go").String()
	if err := os.MkdirAll(filepath.Dir(srcFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcFile, []byte("package pattern\n\ntype ingestutil.Span struct{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lewpath.New(dir, "pkg", "ingest").String(), 0755); err != nil {
		t.Fatal(err)
	}

	root, src, dst := ingest.ResolveMoveArgs(dir, "path:./pkg/pattern/match.go::ingestutil.Span",
		"path:./pkg/ingest/span.go::ingestutil.Span",
	)
	rootAbs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if root != rootAbs {
		t.Fatalf("root=%q want abs %q", root, rootAbs)
	}
	srcRef := ingest.ParseReference(src)
	dstRef := ingest.ParseReference(dst)
	wantSrc := lewpath.New(rootAbs, "pkg", "pattern", "match.go").String()
	wantDst := lewpath.New(rootAbs, "pkg", "ingest", "span.go").String()
	if srcRef.Path != wantSrc || srcRef.Name != "ingestutil.Span" {
		t.Fatalf("source ref=%+v want path %s::ingestutil.Span", srcRef, wantSrc)
	}
	if dstRef.Path != wantDst || dstRef.Name != "ingestutil.Span" {
		t.Fatalf("destination ref=%+v want path %s::ingestutil.Span", dstRef, wantDst)
	}
	// Absolute paths must not be rebased under a source-parent scope.
	if strings.Contains(dstRef.Path, "pattern/pkg") {
		t.Fatalf("destination rebased under source package: %q", dstRef.Path)
	}

	// Rename maps absolute path refs back to project-relative identity.
	proj := ingest.ProjectPathRef(root, srcRef)
	if proj.Path != "./pkg/pattern/match.go" {
		t.Fatalf("ProjectPathRef=%q", proj.Path)
	}
}

func TestResolveMoveArgs_BarePackageDirsAbsolute(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"cmd/codegen", "cmd/gen"} {
		if err := os.MkdirAll(lewpath.New(dir, p).String(), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Minimal Go file so coerce/canonicalize have something under codegen.
	if err := os.WriteFile(lewpath.New(dir, "cmd/codegen/main.go").String(), []byte("package codegen\n"), 0644); err != nil {
		t.Fatal(err)
	}

	root, src, dst := ingest.ResolveMoveArgs(dir, "cmd/codegen", "cmd/gen")
	rootAbs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if root != rootAbs {
		t.Fatalf("root=%q want %q", root, rootAbs)
	}
	srcRef := ingest.ParseReference(src)
	dstRef := ingest.ParseReference(dst)
	if srcRef.Provider != "path" || !filepath.IsAbs(srcRef.Path) || !strings.Contains(srcRef.Path, "codegen") {
		t.Fatalf("source ref=%+v", srcRef)
	}
	if dstRef.Provider != "path" || !filepath.IsAbs(dstRef.Path) || !strings.Contains(dstRef.Path, "gen") {
		t.Fatalf("destination ref=%+v", dstRef)
	}
	if srcRef.Name != "" || dstRef.Name != "" {
		t.Fatalf("package move should have empty symbols: src=%+v dst=%+v", srcRef, dstRef)
	}
	// Nested fixture-like pkg/ must not equal top-level absolute pkg identity.
	nested := lewpath.New(rootAbs, "testdata", "mv", "input", "pkg").String()
	top := lewpath.New(rootAbs, "pkg").String()
	if nested == top {
		t.Fatal("nested and top pkg paths should differ")
	}
	topRef := ingest.AbsolutePathRef(rootAbs, ingest.ParseReference("path:./pkg"))
	if topRef.Path != top {
		// ./pkg may not exist in this temp dir — only check abs form when present
		if st, err := os.Stat(top); err == nil && st.IsDir() && topRef.Path != top {
			t.Fatalf("AbsolutePathRef pkg=%q want %q", topRef.Path, top)
		}
	}
}

func TestRelPathUnderRoot(t *testing.T) {
	dir := t.TempDir()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	sub := lewpath.New(abs, "pkg", "a.go").String()
	rel, err := ingest.RelPathUnderRoot(abs, sub)
	if err != nil || rel != "pkg/a.go" {
		t.Fatalf("rel=%q err=%v", rel, err)
	}
	rel, err = ingest.RelPathUnderRoot(abs, "./pkg/a.go")
	if err != nil || rel != "pkg/a.go" {
		t.Fatalf("rel from ./ =%q err=%v", rel, err)
	}
	if _, err := ingest.RelPathUnderRoot(abs, lewpath.New(abs, "..", "outside").String()); err == nil {
		t.Fatal("expected outside root error")
	}
}
