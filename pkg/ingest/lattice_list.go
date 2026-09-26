package ingest

import (
	"github.com/lewtec/patlint/pkg/project"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// ListAtomMoveNodes lists path-provider atoms in projectFamily.
// skipName, if non-nil, excludes atoms whose name (or leaf) should not be moved.
func ListAtomMoveNodes(result *project.Result, projectFamily string, skipName func(name string) bool) []MoveNode {
	var out []MoveNode
	if result == nil {
		return nil
	}
	langByPath := map[string]string{}
	for _, f := range result.Files {
		langByPath[strings.TrimPrefix(f.Path, "./")] = f.Language
	}
	for _, e := range result.Atoms {
		ref := ParseReference(e.Reference)
		if ref.Provider != "path" || ref.Name == "" {
			continue
		}
		rel := strings.TrimPrefix(ref.Path, "./")
		lang := langByPath[rel]
		if lang == "" || !project.LanguageInFamily(result.Families, lang, projectFamily) {
			continue
		}
		if skipName != nil && skipName(ref.Name) {
			continue
		}
		p := ref.Path
		if !strings.HasPrefix(p, "./") {
			p = "./" + p
		}
		out = append(out, MoveNode{
			Grain:     MoveGrainAtom,
			Reference: e.Reference,
			Path:      p,
			Name:      ref.Name,
		})
	}
	return out
}

// ListPackageMoveNodes lists unique parent directories of family files.
// skipRel, if non-nil, excludes a directory path (no leading ./).
func ListPackageMoveNodes(result *project.Result, projectFamily string, skipRel func(dir string) bool) []MoveNode {
	if result == nil {
		return nil
	}
	dirs := map[string]bool{}
	for _, f := range result.Files {
		if !project.LanguageInFamily(result.Families, f.Language, projectFamily) {
			continue
		}
		rel := strings.TrimPrefix(f.Path, "./")
		if path.Base(rel) == "module-info.java" {
			continue
		}
		dir := path.Dir(rel)
		if dir == "." || dir == "" {
			continue
		}
		if skipRel != nil && skipRel(dir) {
			continue
		}
		dirs[dir] = true
	}
	keys := make([]string, 0, len(dirs))
	for d := range dirs {
		keys = append(keys, d)
	}
	slices.Sort(keys)
	var out []MoveNode
	for _, d := range keys {
		p := "./" + d
		out = append(out, MoveNode{
			Grain:     MoveGrainPackage,
			Reference: "path:" + p,
			Path:      p,
		})
	}
	return out
}

// ListModuleFileMoveNodes lists each source file as a module grain node.
func ListModuleFileMoveNodes(result *project.Result, projectFamily string) []MoveNode {
	if result == nil {
		return nil
	}
	var out []MoveNode
	for _, f := range result.Files {
		if !project.LanguageInFamily(result.Families, f.Language, projectFamily) {
			continue
		}
		p := f.Path
		if !strings.HasPrefix(p, "./") {
			p = "./" + p
		}
		out = append(out, MoveNode{
			Grain:     MoveGrainModule,
			Reference: "path:" + p,
			Path:      p,
		})
	}
	return out
}

// DirModuleKey is the package-directory module key (Go/JVM style).
func DirModuleKey(filePath string) string {
	rel := strings.TrimPrefix(filepath.ToSlash(filePath), "./")
	d := path.Dir(rel)
	if d == "." {
		return ""
	}
	return d
}

// FileModuleKey is the file-as-module key (Python/ECMA style).
func FileModuleKey(filePath string) string {
	return strings.TrimPrefix(filepath.ToSlash(filePath), "./")
}
