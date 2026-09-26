package ingest

import (
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
)

// CoerceLocalPathReference turns provider-less references into path references
// when they point to an existing local filesystem entry under baseDir.
func CoerceLocalPathReference(baseDir string, ref Reference) Reference {
	if baseDir == "" {
		baseDir = "."
	}
	if ref.Provider != "" || ref.Path == "" {
		return ref
	}

	statPath := ref.Path
	if !filepath.IsAbs(statPath) {
		statPath = lewpath.New(baseDir, statPath).String()
	}
	if _, err := os.Stat(statPath); err != nil {
		return ref
	}

	ref.Provider = "path"
	if filepath.IsAbs(ref.Path) || strings.HasPrefix(ref.Path, "./") || strings.HasPrefix(ref.Path, "../") {
		return ref
	}
	ref.Path = "./" + ref.Path
	return ref
}

// CleanRelDir strips a leading "./" and trailing "/" from a relative path.
// Used for directory keys that compare without those decorations.
func CleanRelDir(p string) string {
	return strings.TrimSuffix(strings.TrimPrefix(p, "./"), "/")
}

func pathJoinSlash(dir, base string) string {
	dir = CleanRelDir(dir)
	if dir == "" || dir == "." {
		return base
	}
	return dir + "/" + base
}

// ResolveInputReferenceScope parses input and coerces implicit local paths.
// Definition hops are Walker.CanonicalizeReference (caller).
func ResolveInputReferenceScope(baseDir, input string) ReferenceScope {
	ref := ParseReference(input)
	ref = CoerceLocalPathReference(baseDir, ref)
	return ResolveReferenceScope(baseDir, ref)
}

// ResolveMoveArgs prepares Rename root and source/destination refs from CLI-style
// inputs under project baseDir (typically cwd).
//
// Unlike ResolveInputReferenceScope, this does not narrow the ingest root to the
// source file's parent directory. Path refs are expanded to absolute filesystem
// paths so package identity is not a short relative leaf (e.g. "pkg") that
// collides with nested trees like testdata/.../pkg. Rename maps them back to
// ./rel under root for Result identity via ProjectPathRef.
//
// Barrel hops are Walker.CanonicalizeReference (caller). Destination is only
// coerced and path-normalized: the target file may not exist yet.
func ResolveMoveArgs(baseDir, source, destination string) (root, src, dst string) {
	if baseDir == "" {
		baseDir = "."
	}
	rootAbs, err := filepath.Abs(baseDir)
	if err != nil {
		rootAbs = baseDir
	}

	srcRef := ParseReference(source)
	srcRef = CoerceLocalPathReference(baseDir, srcRef)
	srcRef = NormalizeReferenceForScope(baseDir, baseDir, srcRef)
	srcRef = AbsolutePathRef(baseDir, srcRef)

	dstRef := ParseReference(destination)
	dstRef = CoerceLocalPathReference(baseDir, dstRef)
	dstRef = NormalizeReferenceForScope(baseDir, baseDir, dstRef)
	dstRef = AbsolutePathRef(baseDir, dstRef)

	return rootAbs, srcRef.String(), dstRef.String()
}
