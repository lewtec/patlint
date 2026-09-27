package ingest

import (
	"github.com/lewtec/patlint/pkg/projectfs"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
)

// ReferenceScope is a normalized listing/doc scope tuple.
type ReferenceScope struct {
	Dir       string
	Reference Reference
}

// ResolveReferenceScope computes an ingest scope directory and normalizes
// path references to that scope.
func ResolveReferenceScope(baseDir string, ref Reference) ReferenceScope {
	if baseDir == "" {
		baseDir = "."
	}

	scopeDir := baseDir
	if ref.Provider == "path" {
		p := strings.TrimPrefix(ref.Path, "./")
		targetAbs := absolutePathFromBase(baseDir, ref.Path)
		if st, err := (projectfs.OS{}).Stat(targetAbs); err == nil && st.IsDir() {
			switch {
			case filepath.IsAbs(ref.Path):
				scopeDir = ref.Path
			case p == "" || p == ".":
				scopeDir = baseDir
			default:
				scopeDir = lewpath.New(baseDir, p).String()
			}
		} else if p != "" {
			if filepath.IsAbs(ref.Path) {
				scopeDir = filepath.Dir(ref.Path)
			} else {
				scopeDir = lewpath.New(baseDir, filepath.Dir(p)).String()
			}
		}
	}

	return ReferenceScope{
		Dir:       scopeDir,
		Reference: NormalizeReferenceForScope(baseDir, scopeDir, ref),
	}
}

// NormalizeReferenceForScope rewrites path references to be relative to
// scopeDir when they point inside scopeDir.
func NormalizeReferenceForScope(baseDir, scopeDir string, ref Reference) Reference {
	if ref.Provider != "path" || ref.Path == "" {
		return ref
	}

	scopeAbs, err := filepath.Abs(scopeDir)
	if err != nil {
		return ref
	}

	targetAbs := absolutePathFromBase(baseDir, ref.Path)
	rel, err := filepath.Rel(scopeAbs, targetAbs)
	if err != nil {
		return ref
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ref
	}

	rel = filepath.ToSlash(rel)
	if rel == "." {
		ref.Path = "./"
		return ref
	}
	ref.Path = "./" + rel
	return ref
}

// AbsolutePathReferenceForScope converts scoped path references into absolute
// path targets under scope.Dir.
func AbsolutePathReferenceForScope(scope ReferenceScope) Reference {
	ref := scope.Reference
	if ref.Provider != "path" || ref.Path == "" || filepath.IsAbs(ref.Path) {
		return ref
	}

	rel := strings.TrimPrefix(ref.Path, "./")
	target := scope.Dir
	if rel != "" && rel != "." {
		target = lewpath.New(scope.Dir, filepath.FromSlash(rel)).String()
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return ref
	}
	ref.Path = abs
	return ref
}

// AbsolutePathRef rewrites a path-provider ref so Path is an absolute filesystem
// path under baseDir. Non-path refs and already-absolute paths are unchanged.
// Used at the mv CLI boundary so path identity is not a short relative leaf
// (e.g. "pkg") that collides with nested dirs like testdata/.../pkg.
func AbsolutePathRef(baseDir string, ref Reference) Reference {
	if ref.Provider != "path" || ref.Path == "" || filepath.IsAbs(ref.Path) {
		return ref
	}
	ref.Path = absolutePathFromBase(baseDir, ref.Path)
	return ref
}

// RelPathUnderRoot maps an absolute or ./relative path to a clean slash path
// relative to root (no leading "./"). Empty string means the root itself.
// Paths outside root return an error.
func RelPathUnderRoot(root, p string) (string, error) {
	if p == "" {
		return "", nil
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	var targetAbs string
	if filepath.IsAbs(p) {
		targetAbs = filepath.Clean(p)
	} else {
		targetAbs = absolutePathFromBase(rootAbs, p)
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errPathOutsideRoot(p, rootAbs)
	}
	if rel == "." {
		return "", nil
	}
	return filepath.ToSlash(rel), nil
}

// ProjectPathRef rewrites a path ref to ./rel form under root (ingest Result
// identity). Absolute paths under root become ./rel; others unchanged on error.
func ProjectPathRef(root string, ref Reference) Reference {
	if ref.Provider != "path" || ref.Path == "" {
		return ref
	}
	rel, err := RelPathUnderRoot(root, ref.Path)
	if err != nil {
		return ref
	}
	if rel == "" {
		ref.Path = "./"
		return ref
	}
	ref.Path = "./" + rel
	return ref
}

func errPathOutsideRoot(p, rootAbs string) error {
	return &pathOutsideRootError{path: p, root: rootAbs}
}

type pathOutsideRootError struct {
	path, root string
}

func (e *pathOutsideRootError) Error() string {
	return "path " + e.path + " outside root " + e.root
}

func absolutePathFromBase(baseDir, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	trimmed := strings.TrimPrefix(p, "./")
	joined := lewpath.New(baseDir, trimmed).String()
	abs, err := filepath.Abs(joined)
	if err != nil {
		return joined
	}
	return abs
}
