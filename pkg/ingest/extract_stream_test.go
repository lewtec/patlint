package ingest_test

import (
	"context"
	"errors"
	"os"
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

func TestWalkExtracts_DirStreamsThenMaterialize(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "sub", "b.go").String(), "package p\n\nfunc B() {}\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	err = w.WalkExtracts(t.Context(), ingest.ExtractSource{
		Kind:      ingest.ExtractDir,
		Root:      dir,
		Recursive: true,
	}, func(fe *project.FileExtract) bool {
		n++
		if fe == nil || fe.Language != "go" {
			t.Fatalf("bad extract: %+v", fe)
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("expected >=2 extracts, got %d", n)
	}

	res, err := w.Load(t.Context(), ingest.ExtractSource{
		Kind:      ingest.ExtractDir,
		Root:      dir,
		Recursive: true,
	}, ingest.MaterializeOptions{ExpandImports: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Atoms) < 2 {
		t.Fatalf("expected entities, got %+v", res.Atoms)
	}
}

func TestWalkExtracts_HopSingleFile(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "only.go").String()
	mustWrite(t, path, "package p\n\nfunc Only() {}\n")
	mustWrite(t, lewpath.New(dir, "other.go").String(), "package p\n\nfunc Other() {}\n")

	var paths []string
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WalkExtracts(t.Context(), ingest.ExtractSource{
		Kind:  ingest.ExtractHop,
		Root:  dir,
		Paths: []string{path},
	}, func(fe *project.FileExtract) bool {
		paths = append(paths, fe.Path)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "only.go" {
		t.Fatalf("hop should parse one file, got %v", paths)
	}
}

func TestWalkExtracts_StopEarly(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "b.go").String(), "package p\n\nfunc B() {}\n")

	var n int
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WalkExtracts(t.Context(), ingest.ExtractSource{
		Kind:      ingest.ExtractDir,
		Root:      dir,
		Recursive: true,
	}, func(*project.FileExtract) bool {
		n++
		return false
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("stop early: got %d yields", n)
	}
}

func TestWalkExtracts_ContextCancelBetweenFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "b.go").String(), "package p\n\nfunc B() {}\n")

	ctx, cancel := context.WithCancel(t.Context())
	var n int
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WalkExtracts(ctx, ingest.ExtractSource{
		Kind:      ingest.ExtractDir,
		Root:      dir,
		Recursive: true,
	}, func(*project.FileExtract) bool {
		n++
		cancel() // next file must not start
		return true
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v (n=%d)", err, n)
	}
	if n != 1 {
		t.Fatalf("cancel between files: got %d yields, want 1", n)
	}
}

func TestProjectResult_UsesSpine(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "main.go").String(), "package main\n\nfunc main() {}\n")
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	got, err := w.Load(t.Context(), ingest.SourceProject(dir), ingest.MaterializeOptions{ExpandImports: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Atoms) == 0 {
		t.Fatal("expected entities from ProjectResult")
	}
	_ = os.ErrNotExist
}

func TestSeedResult_BFSNeighbors(t *testing.T) {
	dir := t.TempDir()
	// Two co-located Go files: seed one, BFS should pull the sibling.
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "b.go").String(), "package p\n\nfunc B() {}\n")
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	res, err := w.Load(t.Context(), ingest.SourceSeed(dir, lewpath.New(dir, "a.go").String()), ingest.MaterializeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]bool{}
	for _, e := range res.Atoms {
		refs[e.Reference] = true
	}
	if !refs["path:./a.go::A"] {
		t.Fatalf("missing A: %+v", res.Atoms)
	}
	if !refs["path:./b.go::B"] {
		t.Fatalf("seed BFS should include sibling B: %+v", res.Atoms)
	}
}

func TestLoad_SourceDirNonRecursive(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "root.go").String(), "package p\n\nfunc Root() {}\n")
	mustWrite(t, lewpath.New(dir, "sub", "nested.go").String(), "package p\n\nfunc Nested() {}\n")
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	res, err := w.Load(t.Context(), ingest.SourceDir(dir, "", false), ingest.MaterializeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Atoms {
		if e.Reference == "path:./sub/nested.go::Nested" {
			t.Fatalf("non-recursive should omit nested: %+v", res.Atoms)
		}
	}
	found := false
	for _, e := range res.Atoms {
		if e.Reference == "path:./root.go::Root" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected root entity: %+v", res.Atoms)
	}
}

func TestWalkExtracts_DirRespectsLinguistGenerated(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, ".gitattributes").String(), "*.pb.go linguist-generated\ngen/** linguist-generated\ngen/keep.go -linguist-generated\n")
	mustWrite(t, lewpath.New(dir, "hand.go").String(), "package p\n\nfunc Hand() {}\n")
	mustWrite(t, lewpath.New(dir, "api.pb.go").String(), "package p\n\nfunc Gen() {}\n")
	mustWrite(t, lewpath.New(dir, "gen", "a.go").String(), "package gen\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "gen", "keep.go").String(), "package gen\n\nfunc Keep() {}\n")
	mustWrite(t, lewpath.New(dir, "node_modules", "x", "x.go").String(), "package x\n\nfunc X() {}\n")

	var paths []string
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WalkExtracts(t.Context(), ingest.ExtractSource{
		Kind:      ingest.ExtractDir,
		Root:      dir,
		Recursive: true,
	}, func(fe *project.FileExtract) bool {
		paths = append(paths, fe.Path)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range paths {
		seen[p] = true
	}
	if !seen["hand.go"] {
		t.Fatalf("want hand.go, got %v", paths)
	}
	if !seen["gen/keep.go"] {
		t.Fatalf("want un-ignored gen/keep.go, got %v", paths)
	}
	if seen["api.pb.go"] {
		t.Fatalf("linguist-generated api.pb.go should be skipped, got %v", paths)
	}
	if seen["gen/a.go"] {
		t.Fatalf("linguist-generated gen/a.go should be skipped, got %v", paths)
	}
	for p := range seen {
		if strings.Contains(p, "node_modules") {
			t.Fatalf("node_modules should be skipped, got %v", paths)
		}
	}
}

func TestWalkExtracts_HopIgnoresFilter(t *testing.T) {
	// Explicit hop still parses linguist-generated (user named the file).
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, ".gitattributes").String(), "*.pb.go linguist-generated\n")
	path := lewpath.New(dir, "api.pb.go").String()
	mustWrite(t, path, "package p\n\nfunc Gen() {}\n")

	var n int
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WalkExtracts(t.Context(), ingest.ExtractSource{
		Kind:  ingest.ExtractHop,
		Root:  dir,
		Paths: []string{path},
	}, func(fe *project.FileExtract) bool {
		n++
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("hop should parse generated file when explicit, got %d", n)
	}
}

func TestSeedResult_SkipsGeneratedNeighbors(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, ".gitattributes").String(), "*.pb.go linguist-generated\n")
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "b.pb.go").String(), "package p\n\nfunc B() {}\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	res, err := w.Load(t.Context(), ingest.SourceSeed(dir, lewpath.New(dir, "a.go").String()), ingest.MaterializeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Atoms {
		if strings.Contains(e.Reference, "b.pb.go") {
			t.Fatalf("seed BFS should not pull generated peer: %+v", res.Atoms)
		}
	}
	found := false
	for _, e := range res.Atoms {
		if e.Reference == "path:./a.go::A" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected A: %+v", res.Atoms)
	}
}

func TestPackageSourceFiles_SkipsGeneratedPeers(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, ".gitattributes").String(), "*.pb.go linguist-generated\n")
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "b.pb.go").String(), "package p\n\nfunc B() {}\n")
	a := lewpath.New(dir, "a.go").String()

	// Directory-module file expands to ignore-aware peers (not the generated one).
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	got := w.PackageSourceFiles(t.Context(), a, false)
	if len(got) != 1 || got[0] != a {
		t.Fatalf("file hop peers=%v want only %s", got, a)
	}

	// Explicit dir hop uses the same peer list.
	gotDir := w.PackageSourceFiles(t.Context(), dir, true)
	if len(gotDir) != 1 || gotDir[0] != a {
		t.Fatalf("dir hop peers=%v want only %s", gotDir, a)
	}
}
