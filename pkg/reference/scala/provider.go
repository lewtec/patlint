package scalaref

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lewtec/patlint/pkg/reference/jvmpath"
)

// SymbolTarget points at a directory to ingest for a scala: type reference.
type SymbolTarget struct {
	Dir  string
	Name string
}

// ModuleTarget is a resolved package or compilation unit on disk.
type ModuleTarget struct {
	Dir  string
	File string // basename when the target is a single .scala file
}

// ResolveImport maps a Scala type/package spec to a path: or scala: reference.
// knownFiles keys are slash-separated paths relative to the ingest root.
func ResolveImport(spec string, knownFiles map[string]bool) string {
	spec = normalizeTypeSpec(spec)
	if spec == "" {
		return ""
	}
	if ref, ok := resolveKnownType(spec, knownFiles); ok {
		return ref
	}
	if ref, ok := resolveKnownPackage(spec, knownFiles); ok {
		return ref
	}
	return "scala:" + spec
}

// ResolveTypeFile returns the relative path of the .scala file for spec, if known.
func ResolveTypeFile(spec string, knownFiles map[string]bool) (string, bool) {
	spec = normalizeTypeSpec(spec)
	if spec == "" {
		return "", false
	}
	for _, candidate := range typeFileCandidates(spec) {
		if knownFiles[candidate] {
			return candidate, true
		}
	}
	return "", false
}

// ResolvePackageDir resolves scala:<package> to a filesystem directory under rootDir
// using package-path mapping and common source roots (src/main/scala, src/test/scala, src).
func ResolvePackageDir(spec, rootDir string) (string, error) {
	pkg := normalizeTypeSpec(spec)
	if pkg == "" {
		return "", ErrEmptyPackagePath
	}
	rel := strings.ReplaceAll(pkg, ".", "/")
	for _, dir := range packageDirCandidates(rootDir, rel) {
		st, err := os.Stat(dir)
		if err == nil && st.IsDir() && dirHasScalaSources(dir) {
			return dir, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrPackageNotFound, pkg)
}

// ResolveSymbolTarget resolves scala:<type-or-package>::<symbol>.
func ResolveSymbolTarget(_ context.Context, spec, symbol, rootDir string) (SymbolTarget, bool, error) {
	if symbol == "" {
		return SymbolTarget{}, false, nil
	}
	spec = normalizeTypeSpec(spec)
	if spec == "" {
		return SymbolTarget{}, false, nil
	}

	if file, ok := resolveTypeFileOnDisk(spec, rootDir); ok {
		return SymbolTarget{Dir: filepath.Dir(file), Name: symbol}, true, nil
	}

	pkg := spec
	if idx := strings.LastIndex(spec, "."); idx >= 0 {
		pkg = spec[:idx]
	}
	dir, err := ResolvePackageDir(pkg, rootDir)
	if err != nil {
		dir, err = ResolvePackageDir(spec, rootDir)
		if err != nil {
			return SymbolTarget{}, true, err
		}
	}
	return SymbolTarget{Dir: dir, Name: symbol}, true, nil
}

// ResolveModuleTarget resolves scala:<spec> to a package dir and optional file.
func ResolveModuleTarget(spec, rootDir string) (ModuleTarget, error) {
	spec = normalizeTypeSpec(spec)
	if spec == "" {
		return ModuleTarget{}, ErrEmptyModulePath
	}
	if file, ok := resolveTypeFileOnDisk(spec, rootDir); ok {
		return ModuleTarget{Dir: filepath.Dir(file), File: filepath.Base(file)}, nil
	}
	dir, err := ResolvePackageDir(spec, rootDir)
	if err != nil {
		return ModuleTarget{}, err
	}
	return ModuleTarget{Dir: dir}, nil
}

func resolveKnownType(spec string, knownFiles map[string]bool) (string, bool) {
	if file, ok := ResolveTypeFile(spec, knownFiles); ok {
		return "path:./" + file, true
	}
	return "", false
}

func resolveKnownPackage(spec string, knownFiles map[string]bool) (string, bool) {
	rel := strings.ReplaceAll(spec, ".", "/")
	prefix := rel + "/"
	for file := range knownFiles {
		if file == rel+".scala" {
			continue
		}
		if strings.HasPrefix(file, prefix) && strings.HasSuffix(file, ".scala") {
			return "path:./" + rel, true
		}
		for _, root := range sourceRootPrefixes(file) {
			if strings.HasPrefix(file, root+prefix) && strings.HasSuffix(file, ".scala") {
				return "path:./" + root + rel, true
			}
		}
	}
	return "", false
}

var scalaFileRoots = []string{
	"src/main/scala/", "src/test/scala/",
	"src/main/java/", "src/test/java/",
	"src/",
}
var scalaDirRoots = []string{"", "src/main/scala", "src/test/scala", "src/main/java", "src/test/java", "src"}

func typeFileCandidates(spec string) []string {
	return jvmpath.TypeFileCandidates(spec, ".scala", scalaFileRoots)
}

func sourceRootPrefixes(file string) []string {
	return jvmpath.SourceRootPrefixes(file, scalaFileRoots)
}

func packageDirCandidates(rootDir, rel string) []string {
	return jvmpath.PackageDirCandidates(rootDir, rel, scalaDirRoots)
}

func resolveTypeFileOnDisk(spec, rootDir string) (string, bool) {
	return jvmpath.ResolveTypeFileOnDisk(rootDir, typeFileCandidates(spec))
}

func dirHasScalaSources(dir string) bool {
	return jvmpath.DirHasSources(dir, func(name string) bool {
		return strings.HasSuffix(name, ".scala")
	})
}

func normalizeTypeSpec(spec string) string {
	return jvmpath.NormalizeTypeSpec(spec)
}
