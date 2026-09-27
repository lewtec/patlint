package goref

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"
)

func TestResolveImport_LocalPackageDir(t *testing.T) {
	ref := ResolveImport("helperpkg", map[string]bool{"helperpkg": true})
	require.Equal(t, "path:./helperpkg", ref,
		"unexpected reference: %q", ref)

}

func TestResolveImport_StdlibPathKeepsSlashes(t *testing.T) {
	ref := ResolveImport("net/http", map[string]bool{})
	require.Equal(t, "go:net/http", ref,
		"unexpected reference: %q", ref)

}

func TestResolveSymbolTarget_StdlibPackage(t *testing.T) {
	target, ok, err := ResolveSymbolTarget(t.Context(), "fmt", "Printf", "")
	require.NoError(t, err,
		"resolve symbol target failed: %v", err)
	require.True(t, ok,
		"expected symbol target to resolve")
	require.Equal(t, "Printf", target.Name,
		"unexpected symbol: %q", target.Name)

	suffix := filepath.ToSlash(lewpath.New("src", "fmt").String())
	require.True(t, strings.HasSuffix(filepath.ToSlash(target.Dir), suffix),
		"unexpected target dir: %q", target.Dir)

}

func TestResolvePackageDir_Stdlib(t *testing.T) {
	dir, err := ResolvePackageDir(t.Context(), "fmt", "")
	require.NoError(t, err,
		"resolve package dir failed: %v", err)
	require.True(t, strings.HasSuffix(filepath.ToSlash(dir), filepath.ToSlash(lewpath.New("src", "fmt").String())),
		"unexpected package dir: %q", dir)

}

func TestResolvePackageDir_LocalModuleWithWorkDir(t *testing.T) {
	dir := t.TempDir()
	modName := "example.com/localmod"
	{
		err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module "+modName+"\n\ngo 1.22\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(dir, "main.go").String(), []byte("package main\n\nfunc main() {}\n"), 0o644)
		require.NoError(t, err)
	}

	got, err := ResolvePackageDir(t.Context(), modName, dir)
	require.NoError(t, err,
		"expected local module via workDir: %v", err)
	require.Equal(t, filepath.Clean(dir), filepath.Clean(got),
		"got %q want %q", got, dir)

}

func TestResolveModuleCachePackageDir_Subpackage(t *testing.T) {
	modCache := t.TempDir()
	pkg := lewpath.New(modCache, "github.com", "example", "lib@v1.2.3", "sub", "pkg").String()
	{
		err := os.MkdirAll(pkg, 0755)
		require.NoError(t, err)
	}

	cwd, err := os.Getwd()
	require.NoError(t, err)

	old := os.Getenv("GOMODCACHE")
	t.Cleanup(func() {
		if err := os.Setenv("GOMODCACHE", old); err != nil {
			t.Errorf("restore GOMODCACHE: %v", err)
		}
		if err := os.Chdir(cwd); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	})
	{
		err := os.Setenv("GOMODCACHE", modCache)
		require.NoError(t, err)
	}
	{

		err := os.Chdir(t.TempDir())
		require.NoError(t, err)
	}

	got, ok := resolveModuleCachePackageDir(t.Context(), "github.com/example/lib/sub/pkg")
	require.True(t, ok,
		"expected module-cache resolution")
	require.Equal(t, filepath.Clean(pkg), filepath.Clean(got),
		"unexpected package dir: %q", got)

}
