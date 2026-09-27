package goref

import (
	"bytes"
	"context"
	"fmt"
	"github.com/lewtec/patlint/pkg/projectfs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/reference"
)

// SymbolTarget represents a provider-backed symbol location.
type SymbolTarget struct {
	Dir  string
	Name string
}

// ResolveImport resolves a Go import path into a canonical reference string.
func ResolveImport(spec string, knownDirs map[string]bool) string {
	last := reference.LastPathComponent(spec)
	if knownDirs[last] {
		return "path:./" + last
	}
	return "go:" + spec
}

// ResolveSymbolTarget resolves a go:<pkg>::<symbol> reference to a concrete
// package directory. workDir is the module context (project root) for `go list`.
func ResolveSymbolTarget(ctx context.Context, pkgPath, symbol, workDir string) (SymbolTarget, bool, error) {
	if symbol == "" {
		return SymbolTarget{}, false, nil
	}
	pkgDir, err := ResolvePackageDir(ctx, pkgPath, workDir)
	if err != nil {
		return SymbolTarget{}, true, err
	}

	return SymbolTarget{
		Dir:  pkgDir,
		Name: symbol,
	}, true, nil
}

// ResolvePackageDir resolves a Go import path to a concrete package directory.
// workDir is the directory used as cwd for `go list` so local modules in the
// served/browsed project resolve (e.g. go:workspaced from --dir workspaced).
func ResolvePackageDir(ctx context.Context, pkgPath, workDir string) (string, error) {
	pkgPath = strings.Trim(pkgPath, "/")
	if pkgPath == "" {
		return "", ErrEmptyPackagePath
	}

	if dir, ok := resolveStdlibPackageDir(ctx, pkgPath); ok {
		return dir, nil
	}

	// Prefer go list from the project root so the current module and its
	// replace/workspace deps win over a random module-cache hit.
	if dir, err := goListDir(ctx, workDir, "list", "-f", "{{.Dir}}", pkgPath); err == nil {
		return dir, nil
	}

	if dir, ok := resolveModuleCachePackageDir(ctx, pkgPath); ok {
		return dir, nil
	}

	return "", fmt.Errorf("%w: %s", ErrPackageNotFound, pkgPath)
}

var (
	goRoot     string
	initGoRoot = sync.OnceFunc(func() {
		out, err := exec.Command("go", "env", "GOROOT").Output()
		if err != nil {
			return
		}
		goRoot = strings.TrimSpace(string(out))
	})
)

func goEnvGOROOT(ctx context.Context) string {
	if ctx.Err() != nil {
		return ""
	}
	initGoRoot()
	return goRoot
}

func resolveStdlibPackageDir(ctx context.Context, pkgPath string) (string, bool) {
	root := goEnvGOROOT(ctx)
	if root == "" {
		return "", false
	}
	pkgDir := lewpath.New(root, "src", filepath.FromSlash(pkgPath)).String()
	st, err := (projectfs.OS{}).Stat(pkgDir)
	if err != nil || !st.IsDir() {
		return "", false
	}
	return pkgDir, true
}

func goListDir(ctx context.Context, workDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("%w: %s", ErrPackageNotFound, msg)
		}
		return "", err
	}

	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", ErrEmptyListDir
	}
	st, err := (projectfs.OS{}).Stat(dir)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("%w: %s", ErrInvalidListDir, dir)
	}
	return dir, nil
}

func resolveModuleCachePackageDir(ctx context.Context, pkgPath string) (string, bool) {
	modCache, ok := moduleCacheDir(ctx)
	if !ok {
		return "", false
	}

	parts := strings.Split(pkgPath, "/")
	for i := len(parts); i >= 1; i-- {
		modPath := strings.Join(parts[:i], "/")
		subPath := strings.Join(parts[i:], "/")

		pattern := lewpath.New(modCache, escapeModulePath(modPath)+"@*").String()
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			continue
		}
		slices.Sort(matches)
		for j := len(matches) - 1; j >= 0; j-- {
			candidate := matches[j]
			if subPath != "" {
				candidate = lewpath.New(candidate, filepath.FromSlash(subPath)).String()
			}
			st, err := (projectfs.OS{}).Stat(candidate)
			if err == nil && st.IsDir() {
				return candidate, true
			}
		}
	}

	return "", false
}

func moduleCacheDir(ctx context.Context) (string, bool) {
	if v := strings.TrimSpace(os.Getenv("GOMODCACHE")); v != "" {
		if st, err := (projectfs.OS{}).Stat(v); err == nil && st.IsDir() {
			return v, true
		}
	}

	if v := strings.TrimSpace(os.Getenv("GOPATH")); v != "" {
		for _, gp := range filepath.SplitList(v) {
			if gp == "" {
				continue
			}
			candidate := lewpath.New(gp, "pkg", "mod").String()
			if st, err := (projectfs.OS{}).Stat(candidate); err == nil && st.IsDir() {
				return candidate, true
			}
		}
	}

	if v, err := goListDir(ctx, "", "env", "GOMODCACHE"); err == nil {
		return v, true
	}

	return "", false
}

func escapeModulePath(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			b.WriteByte('!')
			b.WriteByte(c + ('a' - 'A'))
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
