package ingest

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// RewriteMarkedImportPaths edits as-import path tokens in fileRel.
// match returns the replacement specifier for that marked import, or ok=false.
func RewriteMarkedImportPaths(fileRel string, content []byte, result *project.Result, match func(project.Alias) (newSpec string, ok bool)) []project.Edit {
	if result == nil || match == nil || len(content) == 0 {
		return nil
	}
	want := strings.TrimPrefix(filepathToSlashImport(fileRel), "./")
	var edits []project.Edit
	for _, a := range result.Aliases {
		if a.ImportPathStart >= a.ImportPathEnd || int(a.ImportPathEnd) > len(content) {
			continue
		}
		ref := ParseReference(a.Reference)
		if strings.TrimPrefix(filepathToSlashImport(ref.Path), "./") != want {
			continue
		}
		newSpec, ok := match(a)
		if !ok || newSpec == "" {
			continue
		}
		old := string(content[a.ImportPathStart:a.ImportPathEnd])
		if old == newSpec {
			continue
		}
		edits = append(edits, project.Edit{
			File:    strings.TrimPrefix(fileRel, "./"),
			Span:    ingestutil.Span{StartByte: a.ImportPathStart, EndByte: a.ImportPathEnd},
			NewText: newSpec,
		})
	}
	return edits
}

func filepathToSlashImport(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// ImportPathSpan is the as-import path token, then target, then local name.
func ImportPathSpan(imp project.ImportDef) (start, end uint32) {
	if imp.PathEndByte > imp.PathStartByte {
		return imp.PathStartByte, imp.PathEndByte
	}
	if imp.TargetEndByte > imp.TargetStartByte {
		return imp.TargetStartByte, imp.TargetEndByte
	}
	return imp.StartByte, imp.EndByte
}

// RebaseMarkedImports rewrites relative as-import tokens in a file that moved.
func RebaseMarkedImports(sourceRelative, destinationRelative string, content []byte, result *project.Result) []project.Edit {
	if importFileDir(sourceRelative) == importFileDir(destinationRelative) {
		return nil
	}
	return RewriteMarkedImportPaths(sourceRelative, content, result, func(a project.Alias) (string, bool) {
		np := importSpecifier(a.ImportPath).rebase(sourceRelative, destinationRelative)
		if np == "" || np == a.ImportPath {
			return "", false
		}
		return np, true
	})
}

// RewriteMarkedImportDir rewrites marked import specs whose path is oldDir
// (slash or dotted) to newDir. Used by package/directory mv.
func RewriteMarkedImportDir(fileRel string, content []byte, result *project.Result, oldDir, newDir string) []project.Edit {
	return RewriteMarkedImportPaths(fileRel, content, result, func(a project.Alias) (string, bool) {
		np := RewriteImportPathDir(a.ImportPath, oldDir, newDir)
		if np == "" || np == a.ImportPath {
			return "", false
		}
		return np, true
	})
}

// RewriteImportPathFile rewrites a specifier in importerRel that names sourceRelative
// so it names destinationRelative. Preserves ./ and extension style of the original specifier.
func RewriteImportPathFile(importerRel, specifier, sourceRelative, destinationRelative string, policy PackQueries) string {
	if specifier == "" || sourceRelative == "" || destinationRelative == "" {
		return ""
	}
	// Importer-relative resolve only. Absolute dotted modules (pkg.utils)
	// match specNamesFile but are rewritten by destinationModuleSpecifier / dir keys.
	s := importSpecifier(specifier)
	resolved := s.resolveFrom(importerRel)
	if resolved == "" || (!importSpecifier(resolved).namesFile(sourceRelative) && !specifierNamesIndex(resolved, sourceRelative, policy)) {
		return ""
	}
	return s.restyleFor(importerRel, destinationRelative)
}

// importSpecifier is one as-import path token (quoted, ./relative, or dotted module).
type importSpecifier string

// destinationFileRelative is destinationRelative relative to importerRel (slash). No forced "./".
func destinationFileRelative(importerRel, destinationRelative string) string {
	dest := strings.TrimPrefix(filepathToSlashImport(destinationRelative), "./")
	from := importFileDir(importerRel)
	if from == "" {
		return dest
	}
	return relativeSlashPath(from, dest)
}

// RebaseRelativeImport rewrites a relative specifier in a file that moved sourceRelative → destinationRelative.
func RebaseRelativeImport(specifier, sourceRelative, destinationRelative string) string {
	return importSpecifier(specifier).rebase(sourceRelative, destinationRelative)
}

func (s importSpecifier) rebase(sourceRelative, destinationRelative string) string {
	spec := string(s)
	if spec == "" || !s.relative() {
		return ""
	}
	if importFileDir(sourceRelative) == importFileDir(destinationRelative) {
		return ""
	}
	resolved := s.resolveFrom(sourceRelative)
	if resolved == "" {
		return ""
	}
	return s.restyleFor(destinationRelative, resolved)
}

func (s importSpecifier) namesFile(fileRel string) bool {
	a := normalizeFileSpecifier(string(s))
	b := normalizeFileSpecifier(fileRel)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if trimSpecifierExtension(a) == trimSpecifierExtension(b) {
		return true
	}
	a2 := strings.ReplaceAll(trimSpecifierExtension(a), ".", "/")
	b2 := strings.ReplaceAll(trimSpecifierExtension(b), ".", "/")
	return a2 == b2
}

func (s importSpecifier) namesFileFrom(importerRel, fileRel string, policy PackQueries) bool {
	if s.namesFile(fileRel) {
		return true
	}
	resolved := s.resolveFrom(importerRel)
	if resolved == "" {
		return false
	}
	if importSpecifier(resolved).namesFile(fileRel) {
		return true
	}
	return specifierNamesIndex(resolved, fileRel, policy)
}

// RewritePackageJSONPaths rewrites relative path strings in nearby package.json
// files that name sourceRelative so they name destinationRelative (exports/main/module values).
func RewritePackageJSONPaths(dir, sourceRelative, destinationRelative string) []project.Edit {
	var edits []project.Edit
	for _, rel := range packageJSONNear(dir, sourceRelative, destinationRelative) {
		content, err := os.ReadFile(lewpath.New(dir, filepath.FromSlash(rel)).String())
		if err != nil {
			continue
		}
		edits = append(edits, packageJSONPathEdits(rel, content, sourceRelative, destinationRelative)...)
	}
	return edits
}

func packageJSONNear(dir string, rels ...string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimPrefix(filepathToSlashImport(p), "./")
		if p == "" || seen[p] {
			return
		}
		if _, err := os.Stat(lewpath.New(dir, filepath.FromSlash(p)).String()); err != nil {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add("package.json")
	for _, rel := range rels {
		d := importFileDir(rel)
		for {
			if d == "" {
				add("package.json")
				break
			}
			add(path.Join(d, "package.json"))
			parent := path.Dir(d)
			if parent == d || parent == "." {
				add("package.json")
				break
			}
			d = parent
		}
	}
	return out
}

func packageJSONPathEdits(fileRel string, content []byte, sourceRelative, destinationRelative string) []project.Edit {
	var edits []project.Edit
	for i := 0; i < len(content); i++ {
		if content[i] != '"' {
			continue
		}
		start := i + 1
		j := start
		for j < len(content) {
			if content[j] == '\\' {
				j += 2
				continue
			}
			if content[j] == '"' {
				break
			}
			j++
		}
		if j >= len(content) {
			break
		}
		spec := string(content[start:j])
		np := RewriteImportPathFile(fileRel, spec, sourceRelative, destinationRelative, nil)
		if np != "" && np != spec {
			edits = append(edits, project.Edit{
				File:    fileRel,
				Span:    ingestutil.Span{StartByte: uint32(start), EndByte: uint32(j)},
				NewText: np,
			})
		}
		i = j
	}
	return edits
}

func specifierNamesIndex(resolved, fileRel string, policy PackQueries) bool {
	if policy == nil {
		return false
	}
	fileRel = strings.TrimPrefix(filepathToSlashImport(fileRel), "./")
	if !policy.MatchDirectoryRepresentant(fileRel) {
		return false
	}
	dir := path.Dir(fileRel)
	if dir == "." {
		dir = ""
	}
	resolved = strings.TrimPrefix(filepathToSlashImport(resolved), "./")
	return resolved == dir || trimSpecifierExtension(resolved) == dir
}

func specifierLooksRelative(specifier string) bool {
	specifier = strings.TrimSpace(strings.Trim(specifier, `"'`+"`"))
	return specifier == "." || strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") || isDotModuleSpecifier(specifier)
}

func (s importSpecifier) resolveFrom(importerRel string) string {
	spec := strings.TrimSpace(string(s))
	spec = strings.Trim(spec, `"'`+"`")
	if spec == "" {
		return ""
	}
	if isDotModuleSpecifier(spec) {
		return resolveDotModule(importerRel, spec)
	}
	spec = normalizeFileSpecifier(spec)
	from := importFileDir(importerRel)
	slash := spec
	if !looksLikeFileSpecifier(spec) && strings.Contains(spec, ".") && !strings.Contains(spec, "/") {
		slash = strings.ReplaceAll(spec, ".", "/")
	}
	if from == "" {
		return slash
	}
	return path.Clean(path.Join(from, slash))
}

func looksLikeFileSpecifier(specifier string) bool {
	switch path.Ext(normalizeFileSpecifier(specifier)) {
	case ".py", ".js", ".ts", ".tsx", ".jsx", ".mjs", ".cjs", ".zig", ".go", ".json":
		return true
	}
	return false
}

func isDotModuleSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, ".") && !strings.HasPrefix(specifier, "./") && !strings.HasPrefix(specifier, "../")
}

func resolveDotModule(importerRel, specifier string) string {
	from := importFileDir(importerRel)
	ups := 0
	for strings.HasPrefix(specifier, ".") {
		ups++
		specifier = specifier[1:]
	}
	d := from
	for i := 1; i < ups; i++ {
		if d == "" || d == "." {
			break
		}
		d = path.Dir(d)
		if d == "." {
			d = ""
		}
	}
	rest := strings.ReplaceAll(specifier, ".", "/")
	if rest == "" {
		return d
	}
	if d == "" {
		return rest
	}
	return path.Join(d, rest)
}

func dirHasPackageIndex(root, rel string, policy PackQueries) bool {
	if root == "" || policy == nil {
		return false
	}
	d := root
	if rel != "" {
		d = lewpath.New(root, filepath.FromSlash(rel)).String()
	}
	ents, err := os.ReadDir(d)
	if err != nil {
		return false
	}
	rel = strings.TrimPrefix(filepathToSlashImport(rel), "./")
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		childRel := e.Name()
		if rel != "" && rel != "." {
			childRel = path.Join(rel, e.Name())
		}
		if policy.MatchDirectoryRepresentant(childRel) {
			return true
		}
	}
	return false
}

func destinationModuleSpecifier(dir, importerRel, destinationRelative string, policy PackQueries) string {
	dst := strings.TrimPrefix(filepathToSlashImport(destinationRelative), "./")
	dst = trimSpecifierExtension(dst)
	from := importFileDir(importerRel)
	if from == "" {
		if !strings.Contains(dst, "/") {
			if dirHasPackageIndex(dir, "", policy) {
				return "." + dst
			}
			return dst
		}
		return strings.ReplaceAll(dst, "/", ".")
	}
	dstDir := importFileDir(destinationRelative)
	if from != dstDir && !strings.HasPrefix(dst, from+"/") && CommonPathPrefix(from, dstDir) == "" {
		return strings.ReplaceAll(dst, "/", ".")
	}
	rel := relativeSlashPath(from, dst)
	if !strings.HasPrefix(rel, "../") {
		rest := strings.TrimPrefix(rel, "./")
		return "." + strings.ReplaceAll(rest, "/", ".")
	}
	ups := 0
	rest := rel
	for strings.HasPrefix(rest, "../") {
		ups++
		rest = rest[3:]
	}
	return strings.Repeat(".", ups+1) + strings.ReplaceAll(rest, "/", ".")
}

func destinationDotModule(importerRel, destinationRelative string) string {
	dst := strings.TrimPrefix(filepathToSlashImport(destinationRelative), "./")
	dst = trimSpecifierExtension(dst)
	from := importFileDir(importerRel)
	if from == "" {
		return "." + path.Base(dst)
	}
	return destinationModuleSpecifier("", importerRel, destinationRelative, nil)
}

func (s importSpecifier) restyleFor(importerRel, destinationRelative string) string {
	spec := string(s)
	if isDotModuleSpecifier(spec) {
		return destinationDotModule(importerRel, destinationRelative)
	}
	rel := destinationFileRelative(importerRel, destinationRelative)
	if !looksLikeFileSpecifier(spec) {
		rel = trimSpecifierExtension(rel)
		if !strings.HasPrefix(spec, ".") && !strings.Contains(spec, "/") && strings.Contains(spec, ".") {
			return strings.ReplaceAll(strings.TrimPrefix(rel, "./"), "/", ".")
		}
	} else if path.Ext(normalizeFileSpecifier(spec)) == "" {
		rel = trimSpecifierExtension(rel)
	}
	dotted := strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../")
	if dotted {
		if rel != "" && !strings.HasPrefix(rel, ".") {
			return "./" + rel
		}
		return rel
	}
	return strings.TrimPrefix(rel, "./")
}

func (s importSpecifier) relative() bool {
	spec := string(s)
	n := normalizeFileSpecifier(spec)
	if n == "" {
		return false
	}
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return true
	}
	return path.Ext(n) != ""
}

func importFileDir(rel string) string {
	rel = strings.TrimPrefix(filepathToSlashImport(rel), "./")
	if rel == "" || rel == "." {
		return ""
	}
	if path.Ext(rel) == "" {
		return rel
	}
	d := path.Dir(rel)
	if d == "." {
		return ""
	}
	return d
}

func fileStem(rel string) string {
	base := path.Base(strings.TrimPrefix(rel, "./"))
	return trimSpecifierExtension(base)
}

func relativeSlashPath(fromDir, destination string) string {
	fromDir = strings.TrimPrefix(filepathToSlashImport(fromDir), "./")
	destination = strings.TrimPrefix(filepathToSlashImport(destination), "./")
	if fromDir == "" || fromDir == "." {
		return destination
	}
	fromParts := strings.Split(fromDir, "/")
	destParts := strings.Split(destination, "/")
	i := 0
	for i < len(fromParts) && i < len(destParts) && fromParts[i] == destParts[i] {
		i++
	}
	var b []string
	for j := 0; j < len(fromParts)-i; j++ {
		b = append(b, "..")
	}
	b = append(b, destParts[i:]...)
	if len(b) == 0 {
		return "."
	}
	return strings.Join(b, "/")
}

func normalizeFileSpecifier(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"'`+"`")
	p = strings.TrimPrefix(filepathToSlashImport(p), "./")
	return p
}

func trimSpecifierExtension(p string) string {
	if !looksLikeFileSpecifier(p) {
		return p
	}
	return strings.TrimSuffix(p, path.Ext(p))
}

// RewriteImportPathDir replaces oldDir with newDir on a specifier ( / or . ).
func RewriteImportPathDir(specifier, oldDir, newDir string) string {
	if specifier == "" || oldDir == "" || newDir == "" {
		return ""
	}
	for _, sep := range []string{"/", ".", "::"} {
		old := dirKey(oldDir, sep)
		neu := dirKey(newDir, sep)
		if old == "" {
			continue
		}
		if specifier == old {
			return neu
		}
		if strings.HasSuffix(specifier, sep+old) {
			return strings.TrimSuffix(specifier, old) + neu
		}
	}
	return ""
}

func dirKey(dir, sep string) string {
	dir = strings.TrimPrefix(filepathToSlashImport(dir), "./")
	dir = strings.ReplaceAll(dir, "::", "/")
	parts := strings.FieldsFunc(dir, func(r rune) bool { return r == '/' || r == '.' })
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, sep)
}
