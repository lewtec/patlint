package javaref

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"
)

func TestResolveImport_KnownTypeAndPackage(t *testing.T) {
	known := map[string]bool{
		"lib/Helper.java": true,
		"app/Main.java":   true,
	}
	{
		got := ResolveImport("lib.Helper", known)
		require.Equal(t, "path:./lib/Helper.java", got,
			"type ref: got %q", got)
	}
	{

		got := ResolveImport("lib", known)
		require.Equal(t, "path:./lib", got,
			"package ref: got %q", got)
	}

	got := ResolveImport("java.util.List", known)
	require.Equal(t, "java:java.util.List", got,
		"external ref: got %q", got)

}

func TestResolveImport_SourceRoot(t *testing.T) {
	known := map[string]bool{
		"src/main/java/com/example/Helper.java": true,
	}
	{
		got := ResolveImport("com.example.Helper", known)
		require.Equal(t, "path:./src/main/java/com/example/Helper.java", got,
			"got %q", got)
	}

	got := ResolveImport("com.example", known)
	require.Equal(t, "path:./src/main/java/com/example", got,
		"got %q", got)

}

func TestResolvePackageDirAndSymbol(t *testing.T) {
	root := t.TempDir()
	dir := lewpath.New(root, "src", "main", "java", "com", "example").String()
	{
		err := os.MkdirAll(dir, 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(dir, "Helper.java").String(), []byte("package com.example;\npublic class Helper {}\n"), 0o644)
		require.NoError(t, err)
	}

	pkgDir, err := ResolvePackageDir("com.example", root)
	require.NoError(t, err)
	require.Equal(t, dir, pkgDir,
		"package dir: got %q want %q", pkgDir, dir)

	target, ok, err := ResolveSymbolTarget(t.Context(), "com.example.Helper", "Helper", root)
	require.False(t, err != nil || !ok,
		"symbol target: ok=%v err=%v", ok, err)
	require.False(t, target.Dir != dir || target.Name != "Helper",
		"unexpected target %#v", target)

}
