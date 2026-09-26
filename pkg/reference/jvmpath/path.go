// Package jvmpath holds shared JVM package/type path helpers for java/kotlin/scala
// reference providers (source-root candidates, on-disk type resolution).
package jvmpath

import (
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
)

// NormalizeTypeSpec normalizes a dotted or slash-separated type/package spec.
func NormalizeTypeSpec(spec string) string {
	spec = strings.TrimSpace(spec)
	spec = strings.Trim(spec, "/")
	spec = strings.ReplaceAll(spec, "/", ".")
	spec = strings.Trim(spec, ".")
	return spec
}

// TypeFileCandidates lists relative paths for a type under common layouts.
// sourceRoots should use trailing slashes (e.g. "src/main/java/").
func TypeFileCandidates(spec, ext string, sourceRoots []string) []string {
	rel := strings.ReplaceAll(spec, ".", "/") + ext
	out := []string{rel}
	for _, root := range sourceRoots {
		out = append(out, root+rel)
	}
	return out
}

// SourceRootPrefixes returns which of sourceRoots prefix file.
func SourceRootPrefixes(file string, sourceRoots []string) []string {
	var roots []string
	for _, root := range sourceRoots {
		if strings.HasPrefix(file, root) {
			roots = append(roots, root)
		}
	}
	return roots
}

// PackageDirCandidates joins rootDir with each source root (and bare root)
// plus the package-relative path rel (slash form).
// roots are join segments without trailing slash ("" means project root only).
func PackageDirCandidates(rootDir, rel string, roots []string) []string {
	relFS := filepath.FromSlash(rel)
	var out []string
	if rootDir == "" {
		return out
	}
	rootAbs := rootDir
	if abs, err := filepath.Abs(rootDir); err == nil {
		rootAbs = abs
	}
	for _, root := range roots {
		if root == "" {
			out = append(out, lewpath.New(rootAbs, relFS).String())
			continue
		}
		out = append(out, lewpath.New(rootAbs, filepath.FromSlash(root), relFS).String())
	}
	return out
}

// ResolveTypeFileOnDisk returns the first existing candidate under rootDir.
func ResolveTypeFileOnDisk(rootDir string, candidates []string) (string, bool) {
	if rootDir == "" {
		return "", false
	}
	rootAbs := rootDir
	if abs, err := filepath.Abs(rootDir); err == nil {
		rootAbs = abs
	}
	for _, candidate := range candidates {
		path := lewpath.New(rootAbs, filepath.FromSlash(candidate)).String()
		st, err := os.Stat(path)
		if err == nil && !st.IsDir() {
			return path, true
		}
	}
	return "", false
}

// DirHasSources reports whether dir contains a non-directory entry matching isSource.
func DirHasSources(dir string, isSource func(name string) bool) bool {
	if isSource == nil {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if isSource(entry.Name()) {
			return true
		}
	}
	return false
}
