// Package rustref resolves rust: provider paths (crates.io cache / rustup sysroot).
//
// Project-correct versions come from Cargo.lock via Lockfile (TOML). Registry
// directories are then opened as {name}-{version}, not "lexicographically latest".
package rustref

import (
	"context"
	"errors"
	"github.com/lewtec/patlint/pkg/projectfs"
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
)

var (
	ErrEmptyCratePath = errors.New("rust provider path is empty")
	ErrCrateNotFound  = errors.New("rust crate not found")
)

// CrateTarget is a resolved rust: scope on disk.
type CrateTarget struct {
	Dir   string
	File  string
	IsDir bool
}

// SymbolTarget pins a symbol under a crate path.
type SymbolTarget struct {
	Dir  string
	Name string
}

// ResolveCrateTarget maps a rust: path (crate or crate/subpath) to a directory.
//
// rootDir is the project/module context (same role as go list's workDir). When
// set, Cargo.lock is loaded and the locked version is used to open
// ~/.cargo/registry/src/*/{name}-{version}/. std/core/alloc use rustup.
func ResolveCrateTarget(cratePath, rootDir string) (CrateTarget, error) {
	cratePath = strings.TrimSpace(strings.Trim(cratePath, "/"))
	if cratePath == "" {
		return CrateTarget{}, ErrEmptyCratePath
	}
	name := crateNameFromPath(cratePath)
	if name == "" {
		return CrateTarget{}, ErrEmptyCratePath
	}

	// stdlib first (not in Cargo.lock as a normal registry crate).
	switch name {
	case "std", "core", "alloc", "proc_macro", "test":
		if dir, ok := findRustupLib(name); ok {
			return CrateTarget{Dir: dir, IsDir: true}, nil
		}
		return CrateTarget{}, ErrCrateNotFound
	}

	// Prefer locked version from the current project.
	if rootDir != "" {
		if lf, err := LockfileForProject(rootDir); err == nil && lf != nil {
			if ver, ok := lf.Version(name); ok {
				if dir, ok := findRegistryCrateVersion(name, ver); ok {
					return CrateTarget{Dir: dir, IsDir: true}, nil
				}
			}
		}
	}

	return CrateTarget{}, ErrCrateNotFound
}

// ResolveSymbolTarget resolves rust:crate::Symbol to a directory + name.
func ResolveSymbolTarget(_ context.Context, cratePath, symbol, rootDir string) (SymbolTarget, bool, error) {
	t, err := ResolveCrateTarget(cratePath, rootDir)
	if err != nil {
		return SymbolTarget{}, true, err
	}
	return SymbolTarget{Dir: t.Dir, Name: symbol}, true, nil
}

// MatchesEntityPath reports whether entPath lies under target.Dir.
func MatchesEntityPath(target CrateTarget, entPath string) bool {
	if target.Dir == "" {
		return false
	}
	ent, err := filepath.Abs(entPath)
	if err != nil {
		ent = entPath
	}
	dir, err := filepath.Abs(target.Dir)
	if err != nil {
		dir = target.Dir
	}
	return ent == dir || strings.HasPrefix(ent, dir+string(filepath.Separator))
}

func crateNameFromPath(cratePath string) string {
	name := cratePath
	if i := strings.IndexByte(cratePath, '/'); i > 0 {
		name = cratePath[:i]
	}
	if i := strings.Index(name, "::"); i >= 0 {
		name = name[:i]
	}
	return strings.TrimSpace(name)
}

// findRegistryCrateVersion opens registry/src/<reg>/{name}-{version}/.
// Tries both the given name and hyphen/underscore folds for the folder prefix.
func findRegistryCrateVersion(name, version string) (string, bool) {
	if name == "" || version == "" {
		return "", false
	}
	want := []string{
		name + "-" + version,
	}
	// Folder usually matches Cargo.lock package name as-is.
	alt := strings.ReplaceAll(name, "_", "-")
	if alt != name {
		want = append(want, alt+"-"+version)
	}
	alt2 := strings.ReplaceAll(name, "-", "_")
	if alt2 != name {
		want = append(want, alt2+"-"+version)
	}

	srcRoot := cargoRegistrySrc()
	if srcRoot == "" {
		return "", false
	}
	entries, err := (projectfs.OS{}).ReadDir(srcRoot)
	if err != nil {
		return "", false
	}
	wantSet := map[string]bool{}
	for _, w := range want {
		wantSet[w] = true
	}
	for _, reg := range entries {
		if !reg.IsDir() {
			continue
		}
		regDir := lewpath.New(srcRoot, reg.Name()).String()
		for w := range wantSet {
			cand := lewpath.New(regDir, w).String()
			if st, err := (projectfs.OS{}).Stat(cand); err == nil && st.IsDir() {
				return cand, true
			}
		}
	}
	return "", false
}

func cargoRegistrySrc() string {
	cargoHome := os.Getenv("CARGO_HOME")
	if cargoHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		cargoHome = lewpath.New(home, ".cargo").String()
	}
	return lewpath.New(cargoHome, "registry", "src").String()
}

func findRustupLib(name string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	toolchains := lewpath.New(home, ".rustup", "toolchains").String()
	entries, err := (projectfs.OS{}).ReadDir(toolchains)
	if err != nil {
		return "", false
	}
	for _, tc := range entries {
		if !tc.IsDir() {
			continue
		}
		lib := lewpath.New(toolchains, tc.Name(), "lib", "rustlib", "src", "rust", "library", name).String()
		if st, err := (projectfs.OS{}).Stat(lib); err == nil && st.IsDir() {
			return lib, true
		}
	}
	return "", false
}
