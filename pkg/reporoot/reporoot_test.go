package reporoot_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	lewio "github.com/lewtec/lewkit/x/io"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/reporoot"
)

func TestFind_gitDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := lewio.Mkdirp(lewpath.New(root, ".git").String()); err != nil {
		t.Fatal(err)
	}
	nested := lewpath.New(root, "a", "b").String()
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := reporoot.Find(nested)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	want, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Find=%q want %q", got, want)
	}
}

func TestFind_gitFile_worktree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Linked worktree: .git is a plain file.
	if err := os.WriteFile(lewpath.New(root, ".git").String(), []byte("gitdir: /tmp/fake.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := lewpath.New(root, "pkg").String()
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := reporoot.Find(nested)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	want, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Find=%q want %q", got, want)
	}
}

func TestFind_fromFilePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := lewio.Mkdirp(lewpath.New(root, ".git").String()); err != nil {
		t.Fatal(err)
	}
	file := lewpath.New(root, "main.go").String()
	if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := reporoot.Find(file)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	want, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Find=%q want %q", got, want)
	}
}

func TestFind_notFound(t *testing.T) {
	t.Parallel()
	// t.TempDir is usually under /tmp with no .git ancestors. If this host
	// places temps inside a checkout, skip rather than flake.
	root := t.TempDir()
	_, err := reporoot.Find(root)
	if err == nil {
		t.Skip("temp dir resolves to a git root on this host")
	}
	if !errors.Is(err, reporoot.ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

func TestFind_nestedRepoPrefersInner(t *testing.T) {
	t.Parallel()
	outer := t.TempDir()
	if err := lewio.Mkdirp(lewpath.New(outer, ".git").String()); err != nil {
		t.Fatal(err)
	}
	inner := lewpath.New(outer, "vendor", "lib").String()
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := lewio.Mkdirp(lewpath.New(inner, ".git").String()); err != nil {
		t.Fatal(err)
	}
	file := lewpath.New(inner, "x.go").String()
	if err := os.WriteFile(file, []byte("package lib\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := reporoot.Find(file)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	want, err := filepath.Abs(inner)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Find=%q want inner %q", got, want)
	}
}
