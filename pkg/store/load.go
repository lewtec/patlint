package store

import (
	"path"
	"strings"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/reference"
)

// Flags are closed lang columns (SPEC: not new relations).
const (
	FlagPackageScoped = "package_scoped"
	FlagEmptyDir      = "empty_dir"
	FlagNested        = "nested_members"
	FlagIncludeExport = "include_exports"
)

// LoadIngest writes stratum-1 facts from finder output. Store becomes truth.
func LoadIngest(s *Store, extracts []*project.FileExtract, families []project.FamilyClaim, rules func(lang string) project.LanguageRules) {
	if s == nil {
		return
	}
	seenLang := map[string]bool{}
	for _, fe := range extracts {
		if fe == nil {
			continue
		}
		fp := fe.Path
		s.Insert(RelationFile, Tuple{fp, fe.Language, fe.Package})
		if fe.Package != "" || fe.PackageEnd > 0 {
			s.Insert(RelationPackage, Tuple{fp, fe.Package, Itoa(fe.PackageEnd)})
		}
		if fe.DefaultExport != "" {
			s.Insert(RelationDefault, Tuple{fp, fe.DefaultExport})
		}
		for i, sc := range fe.Scopes {
			s.Insert(RelationScope, ScopeRow(fp, i, sc))
		}
		for _, fl := range fe.Flows {
			s.Insert(RelationFlow, FlowRow(fp, fl))
		}
		for _, a := range fe.Atoms {
			s.Insert(RelationAtom, Tuple{fp, a.Name, Itoa(a.StartByte), Itoa(a.EndByte), boolStr(a.Exported), ItoaInt(a.ScopeIdx)})
		}
		WriteDeclares(s, fp)
		for _, im := range fe.Imports {
			star := boolStr(im.MemberName == "*" || im.LocalName == "*")
			s.Insert(RelationImport, Tuple{
				fp, im.LocalName, im.SourcePath, im.MemberName,
				Itoa(im.StartByte), Itoa(im.EndByte),
				Itoa(im.TargetStartByte), Itoa(im.TargetEndByte),
				boolStr(im.HasAliasBinding),
				Itoa(im.PathStartByte), Itoa(im.PathEndByte),
				star,
			})
		}
		for _, re := range fe.Reexports {
			s.Insert(RelationReexport, Tuple{
				fp, re.ExportName, re.SourceName, re.SourcePath,
				boolStr(re.Star), Itoa(re.SourceStartByte), Itoa(re.SourceEndByte),
			})
		}
		for ui, u := range fe.Usages {
			id := ItoaInt(ui)
			s.Append(RelationUse, Tuple{
				fp, u.Name, Itoa(u.StartByte), Itoa(u.EndByte),
				u.Scope, ItoaInt(u.ScopeIdx), ItoaInt(len(u.Prefix)), id,
			})
			for i, p := range u.Prefix {
				s.Append(RelationUseSegment, Tuple{
					fp, id, ItoaInt(i), p.Name, Itoa(p.StartByte), Itoa(p.EndByte),
				})
			}
		}
		if fe.Language != "" && !seenLang[fe.Language] {
			seenLang[fe.Language] = true
			if rules != nil {
				r := rules(fe.Language)
				if r.PackageScopedBareNames {
					s.Insert(RelationLanguage, Tuple{fe.Language, FlagPackageScoped})
				}
				if r.EmptyPackageDirScoped {
					s.Insert(RelationLanguage, Tuple{fe.Language, FlagEmptyDir})
				}
				if r.NestedTypeMembers {
					s.Insert(RelationLanguage, Tuple{fe.Language, FlagNested})
				}
				if r.IncludeFileExportsBare {
					s.Insert(RelationLanguage, Tuple{fe.Language, FlagIncludeExport})
				}
			}
		}
	}
	_ = families
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func pathDot(rel string) string {
	if strings.HasPrefix(rel, "./") || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
		return rel
	}
	return "./" + rel
}

func atomLeaf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

// WriteDeclares inserts RelationDeclares and RelationAtomInFile from RelationAtom / RelationScope.
func WriteDeclares(s *Store, fp string) {
	if s == nil || fp == "" {
		return
	}
	type atom struct {
		name       string
		start, end uint32
		scope      int
	}
	var atoms []atom
	for _, t := range s.Rows(RelationAtom) {
		if len(t) < 6 || !samePath(t[0], fp) {
			continue
		}
		atoms = append(atoms, atom{t[1], Atoi(t[2]), Atoi(t[3]), AtoiInt(t[5])})
	}
	parents := map[int]int{}
	holeOnly := map[int]bool{}
	maxScope := -1
	for _, t := range s.Rows(RelationScope) {
		if len(t) < 5 || !samePath(t[0], fp) {
			continue
		}
		i := AtoiInt(t[1])
		parents[i] = AtoiInt(t[2])
		if len(t) > 5 && t[5] == "1" {
			holeOnly[i] = true
		}
		if i > maxScope {
			maxScope = i
		}
	}
	skipHole := func(idx int) int {
		for n := 0; holeOnly[idx] && n < 32; n++ {
			p, ok := parents[idx]
			if !ok {
				return -1
			}
			idx = p
		}
		return idx
	}
	firstIn := func(scope int, start uint32) bool {
		for _, o := range atoms {
			if o.scope == scope && o.start < start {
				return false
			}
		}
		return true
	}
	declare := func(a atom) int {
		inn := a.scope
		if i := strings.LastIndex(a.name, "."); i > 0 {
			typ, leaf := a.name[:i], a.name[i+1:]
			if leaf == typ || strings.HasSuffix(typ, "."+leaf) {
				return -2
			}
			if inn < 0 || inn > maxScope {
				return -1
			}
			if !firstIn(inn, a.start) {
				return skipHole(inn)
			}
			if p, ok := parents[inn]; ok && p >= 0 {
				return skipHole(p)
			}
			return skipHole(inn)
		}
		if inn < 0 || inn > maxScope {
			return -1
		}
		if firstIn(inn, a.start) {
			if p, ok := parents[inn]; ok {
				return skipHole(p)
			}
			return -1
		}
		return skipHole(inn)
	}
	for _, a := range atoms {
		leaf := atomLeaf(a.name)
		idx := declare(a)
		if idx >= -1 && leaf != "" {
			s.Insert(RelationDeclares, Tuple{fp, ItoaInt(idx), leaf, reference.AtomRef(pathDot(fp), a.name)})
		}
		s.Insert(RelationAtomInFile, Tuple{fp, a.name, reference.AtomRef(pathDot(fp), a.name)})
		if i := strings.LastIndex(a.name, "."); i > 0 {
			s.Insert(RelationAtomInFile, Tuple{fp, a.name[i+1:], reference.AtomRef(pathDot(fp), a.name)})
		}
	}
}

func dirOf(p string) string {
	d := path.Dir(strings.TrimPrefix(p, "./"))
	if d == "." {
		return ""
	}
	return d
}
