package ingest_test

import (
	"context"
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestWalkExtracts_DirStreamsThenMaterialize(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "sub", "b.go").String(), "package p\n\nfunc B() {}\n")

	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	var n int
	err = w.WalkExtracts(t.Context(), ingest.ExtractSource{
		Kind:      ingest.ExtractDir,
		Root:      dir,
		Recursive: true,
	}, func(fe *project.FileExtract) bool {
		n++
		require.False(t, fe == nil || fe.Language != "go",
			"bad extract: %+v", fe)

		return true
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 2,
		"expected >=2 extracts, got %d", n)

	res, err := w.Load(t.Context(), ingest.ExtractSource{
		Kind:      ingest.ExtractDir,
		Root:      dir,
		Recursive: true,
	}, ingest.MaterializeOptions{ExpandImports: true})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(res.Atoms), 2,
		"expected entities, got %+v", res.Atoms)

}

func TestWalkExtracts_HopSingleFile(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "only.go").String()
	mustWrite(t, path, "package p\n\nfunc Only() {}\n")
	mustWrite(t, lewpath.New(dir, "other.go").String(), "package p\n\nfunc Other() {}\n")

	var paths []string
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	err = w.WalkExtracts(t.Context(), ingest.ExtractSource{
		Kind:  ingest.ExtractHop,
		Root:  dir,
		Paths: []string{path},
	}, func(fe *project.FileExtract) bool {
		paths = append(paths, fe.Path)
		return true
	})
	require.NoError(t, err)
	require.False(t, len(paths) != 1 || paths[0] != "only.go",
		"hop should parse one file, got %v", paths)

}

func TestWalkExtracts_StopEarly(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "b.go").String(), "package p\n\nfunc B() {}\n")

	var n int
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	err = w.WalkExtracts(t.Context(), ingest.ExtractSource{
		Kind:      ingest.ExtractDir,
		Root:      dir,
		Recursive: true,
	}, func(*project.FileExtract) bool {
		n++
		return false
	})
	require.NoError(t, err)
	require.Equal(t, 1, n,
		"stop early: got %d yields", n)

}

func TestWalkExtracts_ContextCancelBetweenFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "b.go").String(), "package p\n\nfunc B() {}\n")

	ctx, cancel := context.WithCancel(t.Context())
	var n int
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	err = w.WalkExtracts(ctx, ingest.ExtractSource{
		Kind:      ingest.ExtractDir,
		Root:      dir,
		Recursive: true,
	}, func(*project.FileExtract) bool {
		n++
		cancel() // next file must not start
		return true
	})
	require.ErrorIs(t, err, context.Canceled,
		"want context.Canceled, got %v (n=%d)", err, n)
	require.Equal(t, 1, n,
		"cancel between files: got %d yields, want 1", n)

}

func TestProjectResult_UsesSpine(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "main.go").String(), "package main\n\nfunc main() {}\n")
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	got, err := w.Load(t.Context(), ingest.SourceProject(dir), ingest.MaterializeOptions{ExpandImports: true})
	require.NoError(t, err)
	require.NotEmpty(t, got.Atoms,
		"expected entities from ProjectResult")

	_ = os.ErrNotExist
}

func TestSeedResult_BFSNeighbors(t *testing.T) {
	dir := t.TempDir()
	// Two co-located Go files: seed one, BFS should pull the sibling.
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "b.go").String(), "package p\n\nfunc B() {}\n")
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	res, err := w.Load(t.Context(), ingest.SourceSeed(dir, lewpath.New(dir, "a.go").String()), ingest.MaterializeOptions{})
	require.NoError(t, err)

	refs := map[string]bool{}
	for _, e := range res.Atoms {
		refs[e.Reference] = true
	}
	require.True(t, refs["path:./a.go::A"],
		"missing A: %+v", res.Atoms)
	require.True(t, refs["path:./b.go::B"],
		"seed BFS should include sibling B: %+v", res.Atoms)

}

func TestLoad_SourceDirNonRecursive(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "root.go").String(), "package p\n\nfunc Root() {}\n")
	mustWrite(t, lewpath.New(dir, "sub", "nested.go").String(), "package p\n\nfunc Nested() {}\n")
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	res, err := w.Load(t.Context(), ingest.SourceDir(dir, "", false), ingest.MaterializeOptions{})
	require.NoError(t, err)

	for _, e := range res.Atoms {
		require.NotEqual(t, e.Reference, "path:./sub/nested.go::Nested",
			"non-recursive should omit nested: %+v", res.Atoms)

	}
	found := false
	for _, e := range res.Atoms {
		if e.Reference == "path:./root.go::Root" {
			found = true
		}
	}
	require.True(t, found,
		"expected root entity: %+v", res.Atoms)

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
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	err = w.WalkExtracts(t.Context(), ingest.ExtractSource{
		Kind:      ingest.ExtractDir,
		Root:      dir,
		Recursive: true,
	}, func(fe *project.FileExtract) bool {
		paths = append(paths, fe.Path)
		return true
	})
	require.NoError(t, err)

	seen := map[string]bool{}
	for _, p := range paths {
		seen[p] = true
	}
	require.True(t, seen["hand.go"],
		"want hand.go, got %v", paths)
	require.True(t, seen["gen/keep.go"],
		"want un-ignored gen/keep.go, got %v", paths)
	require.False(t, seen["api.pb.go"],
		"linguist-generated api.pb.go should be skipped, got %v", paths)
	require.False(t, seen["gen/a.go"],
		"linguist-generated gen/a.go should be skipped, got %v", paths)

	for p := range seen {
		require.False(t, strings.Contains(p, "node_modules"),
			"node_modules should be skipped, got %v", paths)

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
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	err = w.WalkExtracts(t.Context(), ingest.ExtractSource{
		Kind:  ingest.ExtractHop,
		Root:  dir,
		Paths: []string{path},
	}, func(fe *project.FileExtract) bool {
		n++
		return true
	})
	require.NoError(t, err)
	require.Equal(t, 1, n,
		"hop should parse generated file when explicit, got %d", n)

}

func TestSeedResult_SkipsGeneratedNeighbors(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, ".gitattributes").String(), "*.pb.go linguist-generated\n")
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "b.pb.go").String(), "package p\n\nfunc B() {}\n")

	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	res, err := w.Load(t.Context(), ingest.SourceSeed(dir, lewpath.New(dir, "a.go").String()), ingest.MaterializeOptions{})
	require.NoError(t, err)

	for _, e := range res.Atoms {
		require.False(t, strings.Contains(e.Reference, "b.pb.go"),
			"seed BFS should not pull generated peer: %+v", res.Atoms)

	}
	found := false
	for _, e := range res.Atoms {
		if e.Reference == "path:./a.go::A" {
			found = true
		}
	}
	require.True(t, found,
		"expected A: %+v", res.Atoms)

}

func TestPackageSourceFiles_SkipsGeneratedPeers(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, ".gitattributes").String(), "*.pb.go linguist-generated\n")
	mustWrite(t, lewpath.New(dir, "a.go").String(), "package p\n\nfunc A() {}\n")
	mustWrite(t, lewpath.New(dir, "b.pb.go").String(), "package p\n\nfunc B() {}\n")
	a := lewpath.New(dir, "a.go").String()

	// Directory-module file expands to ignore-aware peers (not the generated one).
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	got := w.PackageSourceFiles(t.Context(), a, false)
	require.False(t, len(got) != 1 || got[0] != a,
		"file hop peers=%v want only %s", got, a)

	// Explicit dir hop uses the same peer list.
	gotDir := w.PackageSourceFiles(t.Context(), dir, true)
	require.False(t, len(gotDir) != 1 || gotDir[0] != a,
		"dir hop peers=%v want only %s", gotDir, a)

}
