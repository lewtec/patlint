package javaref

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
)

func TestResolveImport_KnownTypeAndPackage(t *testing.T) {
	known := map[string]bool{
		"lib/Helper.java": true,
		"app/Main.java":   true,
	}
	if got := ResolveImport("lib.Helper", known); got != "path:./lib/Helper.java" {
		t.Fatalf("type ref: got %q", got)
	}
	if got := ResolveImport("lib", known); got != "path:./lib" {
		t.Fatalf("package ref: got %q", got)
	}
	if got := ResolveImport("java.util.List", known); got != "java:java.util.List" {
		t.Fatalf("external ref: got %q", got)
	}
}

func TestResolveImport_SourceRoot(t *testing.T) {
	known := map[string]bool{
		"src/main/java/com/example/Helper.java": true,
	}
	if got := ResolveImport("com.example.Helper", known); got != "path:./src/main/java/com/example/Helper.java" {
		t.Fatalf("got %q", got)
	}
	if got := ResolveImport("com.example", known); got != "path:./src/main/java/com/example" {
		t.Fatalf("got %q", got)
	}
}

func TestResolvePackageDirAndSymbol(t *testing.T) {
	root := t.TempDir()
	dir := lewpath.New(root, "src", "main", "java", "com", "example").String()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "Helper.java").String(), []byte("package com.example;\npublic class Helper {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pkgDir, err := ResolvePackageDir("com.example", root)
	if err != nil {
		t.Fatal(err)
	}
	if pkgDir != dir {
		t.Fatalf("package dir: got %q want %q", pkgDir, dir)
	}

	target, ok, err := ResolveSymbolTarget(t.Context(), "com.example.Helper", "Helper", root)
	if err != nil || !ok {
		t.Fatalf("symbol target: ok=%v err=%v", ok, err)
	}
	if target.Dir != dir || target.Name != "Helper" {
		t.Fatalf("unexpected target %#v", target)
	}
}
