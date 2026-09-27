package ingest

import (
	"context"
	"path"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/projectfs"
	refpkg "github.com/lewtec/patlint/pkg/reference"
	"github.com/lewtec/patlint/pkg/sitter"
)

// DirectoryManifestResolve maps absDir to a child entry (package.json main/exports).
// Family knob directory-manifest selects this. Register from the Node resolver.
type DirectoryManifestResolve func(absDir string) (entryRel string, ok bool)

var directoryManifestResolve DirectoryManifestResolve

// RegisterDirectoryManifestResolve sets the family directory-manifest reader.
func RegisterDirectoryManifestResolve(fn DirectoryManifestResolve) {
	directoryManifestResolve = fn
}

// ResolveDirectoryModuleFile maps a directory to its backing file.
// Family directory-manifest (package.json) runs first. Pack
// as-directory-representant globs fill simple cases (__init__.py, index.*).
func ResolveDirectoryModuleFile(policy PackQueries, absDir, dirRel string) (entryRel string, ok bool) {
	if policy == nil {
		return "", false
	}
	if packDirectoryManifest(policy) && directoryManifestResolve != nil {
		if entry, ok := directoryManifestResolve(absDir); ok && entry != "" {
			return entry, true
		}
	}
	return DiskDirRepresentant(policy, absDir, dirRel)
}

func packDirectoryManifest(policy PackQueries) bool {
	for _, c := range policy.Families() {
		if c.Rules.DirectoryManifest {
			return true
		}
	}
	return false
}

// DiskDirRepresentant is the first pack representant child of dirRel that exists on disk.
func DiskDirRepresentant(m refpkg.RepresentantMatcher, absDir, dirRel string) (string, bool) {
	return packDirectoryRepresentant(m, absDir, dirRel)
}

func packDirectoryRepresentant(m refpkg.RepresentantMatcher, absDir, dirRel string) (string, bool) {
	if m == nil || absDir == "" {
		return "", false
	}
	dirRel = strings.TrimPrefix(filepath.ToSlash(dirRel), "./")
	if dirRel == "." {
		dirRel = ""
	}
	for _, g := range m.DirectoryRepresentants() {
		base := representantBase(g)
		if base == "" {
			continue
		}
		childRel := base
		if dirRel != "" {
			childRel = path.Join(dirRel, base)
		}
		if !m.MatchDirectoryRepresentant(childRel) {
			continue
		}
		st, err := (projectfs.OS{}).Stat(lewpath.New(absDir, filepath.FromSlash(base)).String())
		if err != nil || st.IsDir() {
			continue
		}
		return base, true
	}
	return "", false
}

// KnownDirRepresentant is the first pack representant child of dirRel in known.
func KnownDirRepresentant(m refpkg.RepresentantMatcher, dirRel string, known map[string]bool) (string, bool) {
	if m == nil || known == nil {
		return "", false
	}
	dirRel = strings.TrimPrefix(filepath.ToSlash(dirRel), "./")
	if dirRel == "." {
		dirRel = ""
	}
	for _, g := range m.DirectoryRepresentants() {
		base := representantBase(g)
		if base == "" {
			continue
		}
		childRel := base
		if dirRel != "" {
			childRel = path.Join(dirRel, base)
		}
		if !m.MatchDirectoryRepresentant(childRel) || !known[childRel] {
			continue
		}
		return childRel, true
	}
	return "", false
}

func representantBase(glob string) string {
	g := strings.ReplaceAll(glob, "\\", "/")
	base := path.Base(g)
	if base == "" || base == "." || base == ".." || base == "**" {
		return ""
	}
	if strings.ContainsAny(base, "*?[") {
		return ""
	}
	return base
}

// LocalBindingsForLanguage builds a BindingIndex from pack extract (atoms + uses).
func LocalBindingsForLanguage(ctx context.Context, policy PackQueries, sess *project.Session, root *sitter.Node, source []byte, lang, filePath string) *BindingIndex {
	if policy == nil || sess == nil {
		return nil
	}
	fe, err := ExtractFile(ctx, policy, sess, lang, root, source, filePath)
	if err != nil || fe == nil {
		return nil
	}
	return BindingIndexFromExtract(fe)
}
