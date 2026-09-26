package ingest_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestTargetMatchesPackageSymbol_ModulePathExact(t *testing.T) {
	// Go matching uses PackageImportMatcher (ingest/go) and go.mod at rootDir.
	dir := t.TempDir()
	err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example.com/mod\n\ngo 1.22\n"), 0644)
	require.NoError(t, err)
	require.True(t, // Nested package matches full import path only.
		ingest.TargetMatchesPackageSymbolForTest(dir, ingest.ParseReference("go:example.com/mod/pkg/db::Open"), "pkg/db"),
		"expected go:example.com/mod/pkg/db to match pkg/db under module")
	// Suffix collision: package dir "db" must not match .../pkg/db.
	require.False(t, ingest.TargetMatchesPackageSymbolForTest(dir, ingest.ParseReference("go:example.com/mod/pkg/db::Open"), "db"),
		"pkg/db must not match short pkgDir db")
	// Correct short package at module root segment.
	require.True(t, ingest.TargetMatchesPackageSymbolForTest(dir, ingest.ParseReference("go:example.com/mod/db::Open"), "db"),
		"expected go:example.com/mod/db to match pkgDir db")
	// Empty pkgDir is module root only — not fmt/os single-segment stdlib.
	require.True(t, ingest.TargetMatchesPackageSymbolForTest(dir, ingest.ParseReference("go:example.com/mod::Printf"), ""),
		"root package should match module path exactly")
	require.False(t, ingest.TargetMatchesPackageSymbolForTest(dir, ingest.ParseReference("go:fmt::Printf"), ""),
		"empty pkgDir must not match go:fmt")
	require.False(t, ingest.TargetMatchesPackageSymbolForTest(dir, ingest.ParseReference("go:os::Open"), ""),
		"empty pkgDir must not match go:os")
	// path: still matches by file directory (core; no go.mod needed).
	require.True(t, ingest.TargetMatchesPackageSymbolForTest(dir, ingest.ParseReference("path:./pkg/db/db.go::Open"), "pkg/db"),
		"path entity dir should match")

}

func TestTargetMatchesPackageSymbol_NoModulePath(t *testing.T) {
	dir := t.TempDir()
	require.False(t, ingest.TargetMatchesPackageSymbolForTest(dir, ingest.ParseReference("go:fmt::Printf"), ""),
		"empty pkgDir without module must not match single-segment")
	require.False(t, ingest.TargetMatchesPackageSymbolForTest(dir, ingest.ParseReference("go:example.com/mod/pkg/db::Open"), "db"),
		"suffix match must not apply for short pkgDir against nested path")
	require.True(t, ingest.TargetMatchesPackageSymbolForTest(dir, ingest.ParseReference("go:pkg/db::Open"), "pkg/db"),
		"exact pkgDir equality should still match")

}

func TestExpandRenameSourceSet_TypeUseSameLeaf(t *testing.T) {
	result := &project.Result{
		Atoms: []project.Atom{
			{Reference: "path:./Worker.java::Worker"},
			{Reference: "path:./Worker.java::Worker.work"},
			{Reference: "path:./Task.java::Task"},
			{Reference: "path:./Task.java::Task.work"},
			{Reference: "path:./Other.java::Other.work"},
			{Reference: "path:./Box.java::Box.use.helper"},
			{Reference: "path:./Box.java::use.Local"},
			{Reference: "path:./Box.java::Box.Local.helper"},
		},
		Uses: []project.Use{
			{Reference: "path:./Task.java::Task", Target: "path:./Worker.java::Worker"},
			{Reference: "path:./Box.java::Box.use", Target: "path:./Worker.java::Worker"},
			{Reference: "path:./Box.java::use.Local", Target: "path:./Worker.java::Worker"},
		},
	}
	set := ingest.ExpandRenameSourceSetForTest(".", result, []string{"path:./Worker.java::Worker.work"})
	require.True(t, set.Has("path:./Task.java::Task.work"),
		"Task.work: %v", set)
	require.False(t, set.Has("path:./Other.java::Other.work"),
		"Other.work must stay out")

	set = ingest.ExpandRenameSourceSetForTest(".", result, []string{"path:./Worker.java::Worker.helper"})
	require.True(t, set.Has("path:./Box.java::Box.use.helper"),
		"Box.use.helper: %v", set)
	require.True(t, set.Has("path:./Box.java::Box.Local.helper"),
		"Box.Local.helper: %v", set)

}

func TestExpandRenameSourceSet_UsesModuleImportPath(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example\n\ngo 1.22\n"), 0644)
	require.NoError(t, err)

	result := &project.Result{
		Uses: []project.Use{
			{Target: "go:example/pkg/db::FromContext"},
			{Target: "go:example/other/db::FromContext"}, // wrong package, same leaf
			{Target: "go:fmt::FromContext"},              // single-segment false friend
		},
	}
	set := ingest.ExpandRenameSourceSetForTest(dir, result, []string{"path:./pkg/db/db.go::FromContext"})
	require.True(t, set.Has("go:example/pkg/db::FromContext"),
		"expected go:example/pkg/db::FromContext in set, got %v", set)
	require.False(t, set.Has("go:example/other/db::FromContext"),
		"must not expand other/db via trailing /db suffix")
	require.False(t, set.Has("go:fmt::FromContext"),
		"must not expand stdlib single-segment")

}
