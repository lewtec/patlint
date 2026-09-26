package nixref

import (
	"os"
	"path/filepath"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"
)

func TestResolveTarget_DirectoryUsesDefaultNixAsBackingFile(t *testing.T) {
	root := t.TempDir()
	libDir := lewpath.New(root, "lib").String()
	{
		err := os.MkdirAll(libDir, 0755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(libDir, "default.nix").String(), []byte("{\n  id = x: x;\n}\n"), 0644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(libDir, "extra.nix").String(), []byte("{ }\n"), 0644)
		require.NoError(t, err)
	}

	old := os.Getenv("NIX_PATH")
	t.Cleanup(func() {
		if err := os.Setenv("NIX_PATH", old); err != nil {
			t.Errorf("restore NIX_PATH: %v", err)
		}
	})
	{
		err := os.Setenv("NIX_PATH", "nixpkgs="+root)
		require.NoError(t, err)
	}

	target, err := ResolveTarget("nixpkgs/lib")
	require.NoError(t, err,
		"resolve target failed: %v", err)
	require.Equal(t, filepath.Clean(libDir), filepath.Clean(target.Dir),
		"unexpected dir: got %q want %q", target.Dir, libDir)
	require.Equal(t, "default.nix", target.File,
		"unexpected backing file: got %q want %q", target.File, "default.nix")
	require.True(t, target.IsDir,
		"expected directory target")
	require.True(t, MatchesEntityPath(target, "default.nix"),
		"expected default.nix to match directory-backed nix target")
	require.False(t, MatchesEntityPath(target, "extra.nix"),
		"did not expect sibling nix file to match directory-backed nix target")

}
