package nixref

import (
	"os"
	"path/filepath"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
)

func TestResolveTarget_DirectoryUsesDefaultNixAsBackingFile(t *testing.T) {
	root := t.TempDir()
	libDir := lewpath.New(root, "lib").String()
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(libDir, "default.nix").String(), []byte("{\n  id = x: x;\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(libDir, "extra.nix").String(), []byte("{ }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	old := os.Getenv("NIX_PATH")
	t.Cleanup(func() {
		if err := os.Setenv("NIX_PATH", old); err != nil {
			t.Errorf("restore NIX_PATH: %v", err)
		}
	})
	if err := os.Setenv("NIX_PATH", "nixpkgs="+root); err != nil {
		t.Fatal(err)
	}

	target, err := ResolveTarget("nixpkgs/lib")
	if err != nil {
		t.Fatalf("resolve target failed: %v", err)
	}
	if filepath.Clean(target.Dir) != filepath.Clean(libDir) {
		t.Fatalf("unexpected dir: got %q want %q", target.Dir, libDir)
	}
	if target.File != "default.nix" {
		t.Fatalf("unexpected backing file: got %q want %q", target.File, "default.nix")
	}
	if !target.IsDir {
		t.Fatal("expected directory target")
	}

	if !MatchesEntityPath(target, "default.nix") {
		t.Fatal("expected default.nix to match directory-backed nix target")
	}
	if MatchesEntityPath(target, "extra.nix") {
		t.Fatal("did not expect sibling nix file to match directory-backed nix target")
	}
}
