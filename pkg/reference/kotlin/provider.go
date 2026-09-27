package kotlinref

import (
	"context"
	"fmt"
	"github.com/lewtec/patlint/pkg/projectfs"
	"path/filepath"
	"strings"

	"github.com/lewtec/patlint/pkg/reference/jvmpath"
)

// SymbolTarget points at a directory to ingest for a kotlin: type reference.
type SymbolTarget struct {
	Dir  string
	Name string
}

// ModuleTarget is a resolved package or compilation unit on disk.
type ModuleTarget struct {
	Dir  string
	File string // basename when the target is a single .kt/.kts file
}

// ResolveImport maps a Kotlin type/package spec to a path: or kotlin: reference.
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
	return "kotlin:" + spec
}

// ResolveTypeFile returns the relative path of the .kt/.kts file for spec, if known.
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

// ResolvePackageDir resolves kotlin:<package> to a filesystem directory under rootDir.
func ResolvePackageDir(spec, rootDir string) (string, error) {
	pkg := normalizeTypeSpec(spec)
	if pkg == "" {
		return "", ErrEmptyPackagePath
	}
	rel := strings.ReplaceAll(pkg, ".", "/")
	for _, dir := range packageDirCandidates(rootDir, rel) {
		st, err := (projectfs.OS{}).Stat(dir)
		if err == nil && st.IsDir() && dirHasKotlinSources(dir) {
			return dir, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrPackageNotFound, pkg)
}

// ResolveSymbolTarget resolves kotlin:<type-or-package>::<symbol>.
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

// ResolveModuleTarget resolves kotlin:<spec> to a package dir and optional file.
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
		if file == rel+".kt" || file == rel+".kts" {
			continue
		}
		if strings.HasPrefix(file, prefix) && isKotlinSource(file) {
			return "path:./" + rel, true
		}
		for _, root := range sourceRootPrefixes(file) {
			if strings.HasPrefix(file, root+prefix) && isKotlinSource(file) {
				return "path:./" + root + rel, true
			}
		}
	}
	return "", false
}

var kotlinFileRoots = []string{
	"src/commonMain/kotlin/", "src/commonTest/kotlin/",
	"src/jvmMain/kotlin/", "src/jvmTest/kotlin/",
	"src/main/kotlin/", "src/test/kotlin/",
	"src/main/java/", "src/test/java/",
	"src/",
}
var kotlinDirRoots = []string{
	"", "src/commonMain/kotlin", "src/jvmMain/kotlin",
	"src/main/kotlin", "src/test/kotlin",
	"src/main/java", "src/test/java", "src",
}

func typeFileCandidates(spec string) []string {
	var out []string
	for _, ext := range []string{".kt", ".kts"} {
		out = append(out, jvmpath.TypeFileCandidates(spec, ext, kotlinFileRoots)...)
	}
	return out
}

func sourceRootPrefixes(file string) []string {
	return jvmpath.SourceRootPrefixes(file, kotlinFileRoots)
}

func packageDirCandidates(rootDir, rel string) []string {
	return jvmpath.PackageDirCandidates(rootDir, rel, kotlinDirRoots)
}

func resolveTypeFileOnDisk(spec, rootDir string) (string, bool) {
	return jvmpath.ResolveTypeFileOnDisk(rootDir, typeFileCandidates(spec))
}

func dirHasKotlinSources(dir string) bool {
	return jvmpath.DirHasSources(dir, isKotlinSource)
}

func isKotlinSource(name string) bool {
	return strings.HasSuffix(name, ".kt") || strings.HasSuffix(name, ".kts")
}

func normalizeTypeSpec(spec string) string {
	return jvmpath.NormalizeTypeSpec(spec)
}
