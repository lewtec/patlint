package reporoot_test

import (
	"os"
	"path/filepath"
	"testing"

	lewio "github.com/lewtec/lewkit/x/io"
	"github.com/stretchr/testify/require"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/reporoot"
)

func TestFind_gitDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	{
		err := lewio.Mkdirp(lewpath.New(root, ".git").String())
		require.NoError(t, err)
	}

	nested := lewpath.New(root, "a", "b").String()
	{
		err := os.MkdirAll(nested, 0o755)
		require.NoError(t, err)
	}

	got, err := reporoot.Find(nested)
	require.NoError(t, err,
		"Find: %v", err)

	want, err := filepath.Abs(root)
	require.NoError(t, err)
	require.Equal(t, want, got,
		"Find=%q want %q", got, want)

}

func TestFind_gitFile_worktree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	{
		// Linked worktree: .git is a plain file.
		err := os.WriteFile(lewpath.New(root, ".git").String(), []byte("gitdir: /tmp/fake.git\n"), 0o644)
		require.NoError(t, err)
	}

	nested := lewpath.New(root, "pkg").String()
	{
		err := os.MkdirAll(nested, 0o755)
		require.NoError(t, err)
	}

	got, err := reporoot.Find(nested)
	require.NoError(t, err,
		"Find: %v", err)

	want, err := filepath.Abs(root)
	require.NoError(t, err)
	require.Equal(t, want, got,
		"Find=%q want %q", got, want)

}

func TestFind_fromFilePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	{
		err := lewio.Mkdirp(lewpath.New(root, ".git").String())
		require.NoError(t, err)
	}

	file := lewpath.New(root, "main.go").String()
	{
		err := os.WriteFile(file, []byte("package main\n"), 0o644)
		require.NoError(t, err)
	}

	got, err := reporoot.Find(file)
	require.NoError(t, err,
		"Find: %v", err)

	want, err := filepath.Abs(root)
	require.NoError(t, err)
	require.Equal(t, want, got,
		"Find=%q want %q", got, want)

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
	require.ErrorIs(t, err, reporoot.ErrNotFound,
		"err=%v want ErrNotFound", err)

}

func TestFind_nestedRepoPrefersInner(t *testing.T) {
	t.Parallel()
	outer := t.TempDir()
	{
		err := lewio.Mkdirp(lewpath.New(outer, ".git").String())
		require.NoError(t, err)
	}

	inner := lewpath.New(outer, "vendor", "lib").String()
	{
		err := os.MkdirAll(inner, 0o755)
		require.NoError(t, err)
	}
	{

		err := lewio.Mkdirp(lewpath.New(inner, ".git").String())
		require.NoError(t, err)
	}

	file := lewpath.New(inner, "x.go").String()
	{
		err := os.WriteFile(file, []byte("package lib\n"), 0o644)
		require.NoError(t, err)
	}

	got, err := reporoot.Find(file)
	require.NoError(t, err,
		"Find: %v", err)

	want, err := filepath.Abs(inner)
	require.NoError(t, err)
	require.Equal(t, want, got,
		"Find=%q want inner %q", got, want)

}
