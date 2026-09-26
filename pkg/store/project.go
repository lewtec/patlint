package store

import (
	"sort"
	"strings"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/reference"
)

// FlowRow is RelationFlow (file, start, end, class).
func FlowRow(fp string, fl project.FlowDef) Tuple {
	return Tuple{fp, Itoa(fl.StartByte), Itoa(fl.EndByte), fl.Class}
}

// FlowFromRow reads RelationFlow.
func FlowFromRow(t Tuple) project.FlowDef {
	if len(t) < 4 {
		return project.FlowDef{}
	}
	return project.FlowDef{StartByte: Atoi(t[1]), EndByte: Atoi(t[2]), Class: t[3]}
}

// ScopeRow is RelationScope (file, idx, parent, start, end, hole).
func ScopeRow(fp string, i int, sc project.ScopeDef) Tuple {
	hole := "0"
	if sc.HoleOnly {
		hole = "1"
	}
	return Tuple{fp, ItoaInt(i), ItoaInt(sc.Parent), Itoa(sc.StartByte), Itoa(sc.EndByte), hole}
}

// ScopeFromRow reads RelationScope extras (col 5 optional).
func ScopeFromRow(t Tuple) project.ScopeDef {
	if len(t) < 5 {
		return project.ScopeDef{}
	}
	sc := project.ScopeDef{
		Parent:    AtoiInt(t[2]),
		StartByte: Atoi(t[3]),
		EndByte:   Atoi(t[4]),
	}
	if len(t) > 5 {
		sc.HoleOnly = t[5] == "1"
	}
	return sc
}

// Project is the Result view of a closed store.
func Project(s *Store, families []project.FamilyClaim) *project.Result {
	if s == nil {
		return &project.Result{}
	}
	res := &project.Result{Families: families}
	scopes := map[string][]project.ScopeDef{}
	flows := map[string][]project.FlowDef{}
	for _, t := range s.Rows(RelationFlow) {
		if len(t) < 4 {
			continue
		}
		flows[t[0]] = append(flows[t[0]], FlowFromRow(t))
	}
	for _, t := range s.Rows(RelationScope) {
		if len(t) < 5 {
			continue
		}
		idx := AtoiInt(t[1])
		p := t[0]
		for len(scopes[p]) <= idx {
			scopes[p] = append(scopes[p], project.ScopeDef{})
		}
		scopes[p][idx] = ScopeFromRow(t)
	}
	for _, t := range s.Rows(RelationFile) {
		if len(t) < 2 {
			continue
		}
		res.Files = append(res.Files, project.File{
			Path:     t[0],
			Language: t[1],
			Scopes:   scopes[t[0]],
			Flows:    flows[t[0]],
		})
	}
	for _, t := range s.Rows(RelationPackage) {
		if len(t) < 3 {
			continue
		}
		for i := range res.Files {
			if samePath(res.Files[i].Path, t[0]) {
				res.Files[i].Package = t[1]
				res.Files[i].PackageEnd = Atoi(t[2])
			}
		}
	}
	for _, t := range s.Rows(RelationAtom) {
		if len(t) < 6 {
			continue
		}
		res.Atoms = append(res.Atoms, project.Atom{
			Reference: reference.AtomRef(pathDot(t[0]), t[1]),
			StartByte: Atoi(t[2]),
			EndByte:   Atoi(t[3]),
			Exported:  t[4] == "1",
			ScopeIdx:  AtoiInt(t[5]),
		})
	}
	for _, t := range s.Rows(RelationAlias) {
		if len(t) < 7 {
			continue
		}
		res.Aliases = append(res.Aliases, project.Alias{
			Reference:       t[0],
			StartByte:       Atoi(t[1]),
			EndByte:         Atoi(t[2]),
			Target:          t[3],
			ImportPath:      t[4],
			ImportPathStart: Atoi(t[5]),
			ImportPathEnd:   Atoi(t[6]),
		})
	}
	for _, t := range s.Rows(RelationBinds) {
		if len(t) < 6 {
			continue
		}
		res.Uses = append(res.Uses, project.Use{
			Reference:      t[3],
			StartByte:      Atoi(t[1]),
			EndByte:        Atoi(t[2]),
			Target:         t[4],
			ViaImportAlias: t[5] == "1",
		})
	}
	sort.Slice(res.Files, func(i, j int) bool { return res.Files[i].Path < res.Files[j].Path })
	sort.Slice(res.Atoms, func(i, j int) bool {
		if res.Atoms[i].Reference != res.Atoms[j].Reference {
			return res.Atoms[i].Reference < res.Atoms[j].Reference
		}
		return res.Atoms[i].StartByte < res.Atoms[j].StartByte
	})
	sort.Slice(res.Uses, func(i, j int) bool {
		if res.Uses[i].Reference != res.Uses[j].Reference {
			return res.Uses[i].Reference < res.Uses[j].Reference
		}
		if res.Uses[i].StartByte != res.Uses[j].StartByte {
			return res.Uses[i].StartByte < res.Uses[j].StartByte
		}
		return res.Uses[i].EndByte < res.Uses[j].EndByte
	})
	_ = strings.TrimPrefix
	return res
}

// LoadProjected writes atom / binds / alias facts from a Result view.
func LoadProjected(s *Store, res *project.Result) {
	if s == nil || res == nil {
		return
	}
	for _, a := range res.Atoms {
		r := reference.Parse(a.Reference)
		exp := "0"
		if a.Exported {
			exp = "1"
		}
		s.Insert(RelationAtom, Tuple{r.Path, r.Name, Itoa(a.StartByte), Itoa(a.EndByte), exp, ItoaInt(a.ScopeIdx)})
		if a.Reference != "" {
			s.Insert(RelationAtomInFile, Tuple{r.Path, r.Name, a.Reference})
		}
	}
	for _, al := range res.Aliases {
		s.Insert(RelationAlias, Tuple{
			al.Reference, Itoa(al.StartByte), Itoa(al.EndByte), al.Target,
			al.ImportPath, Itoa(al.ImportPathStart), Itoa(al.ImportPathEnd),
		})
	}
	for _, u := range res.Uses {
		via := "0"
		if u.ViaImportAlias {
			via = "1"
		}
		f := reference.Parse(u.Reference).Path
		s.Append(RelationBinds, Tuple{f, Itoa(u.StartByte), Itoa(u.EndByte), u.Reference, u.Target, via})
	}
}

func samePath(a, b string) bool {
	return a == b || pathDot(a) == pathDot(b)
}

// ProjectExtract is the FileExtract view of one file. Store is truth.
func ProjectExtract(s *Store, path string) *project.FileExtract {
	if s == nil || path == "" {
		return nil
	}
	fe := &project.FileExtract{Path: path}
	for _, t := range s.Rows(RelationFile) {
		if len(t) < 2 || !samePath(t[0], path) {
			continue
		}
		fe.Path = t[0]
		fe.Language = t[1]
		if len(t) > 2 {
			fe.Package = t[2]
		}
	}
	for _, t := range s.Rows(RelationPackage) {
		if len(t) < 3 || !samePath(t[0], path) {
			continue
		}
		fe.Package = t[1]
		fe.PackageEnd = Atoi(t[2])
	}
	for _, t := range s.Rows(RelationDefault) {
		if len(t) < 2 || !samePath(t[0], path) {
			continue
		}
		fe.DefaultExport = t[1]
	}
	maxScope := -1
	for _, t := range s.Rows(RelationScope) {
		if len(t) < 5 || !samePath(t[0], path) {
			continue
		}
		if i := AtoiInt(t[1]); i > maxScope {
			maxScope = i
		}
	}
	if maxScope >= 0 {
		fe.Scopes = make([]project.ScopeDef, maxScope+1)
		for _, t := range s.Rows(RelationScope) {
			if len(t) < 5 || !samePath(t[0], path) {
				continue
			}
			i := AtoiInt(t[1])
			fe.Scopes[i] = ScopeFromRow(t)
		}
	}
	for _, t := range s.Rows(RelationFlow) {
		if len(t) < 4 || !samePath(t[0], path) {
			continue
		}
		fe.Flows = append(fe.Flows, FlowFromRow(t))
	}
	for _, t := range s.Rows(RelationAtom) {
		if len(t) < 6 || !samePath(t[0], path) {
			continue
		}
		fe.Atoms = append(fe.Atoms, project.AtomDef{
			Name:      t[1],
			StartByte: Atoi(t[2]),
			EndByte:   Atoi(t[3]),
			Exported:  t[4] == "1",
			ScopeIdx:  AtoiInt(t[5]),
		})
	}
	for _, t := range s.Rows(RelationImport) {
		if len(t) < 12 || !samePath(t[0], path) {
			continue
		}
		fe.Imports = append(fe.Imports, project.ImportDef{
			LocalName:       t[1],
			SourcePath:      t[2],
			MemberName:      t[3],
			StartByte:       Atoi(t[4]),
			EndByte:         Atoi(t[5]),
			TargetStartByte: Atoi(t[6]),
			TargetEndByte:   Atoi(t[7]),
			HasAliasBinding: t[8] == "1",
			PathStartByte:   Atoi(t[9]),
			PathEndByte:     Atoi(t[10]),
		})
	}
	for _, t := range s.Rows(RelationReexport) {
		if len(t) < 7 || !samePath(t[0], path) {
			continue
		}
		fe.Reexports = append(fe.Reexports, project.ReexportDef{
			ExportName:      t[1],
			SourceName:      t[2],
			SourcePath:      t[3],
			Star:            t[4] == "1",
			SourceStartByte: Atoi(t[5]),
			SourceEndByte:   Atoi(t[6]),
		})
	}
	type segs []project.UsageName
	byID := map[string]segs{}
	for _, t := range s.Rows(RelationUseSegment) {
		if len(t) < 6 || !samePath(t[0], path) {
			continue
		}
		id, idx := t[1], AtoiInt(t[2])
		sl := byID[id]
		for len(sl) <= idx {
			sl = append(sl, project.UsageName{})
		}
		sl[idx] = project.UsageName{Name: t[3], StartByte: Atoi(t[4]), EndByte: Atoi(t[5])}
		byID[id] = sl
	}
	for _, t := range s.Rows(RelationUse) {
		if len(t) < 7 || !samePath(t[0], path) {
			continue
		}
		id := "0"
		if len(t) > 7 {
			id = t[7]
		}
		fe.Usages = append(fe.Usages, project.UsageDef{
			Name:      t[1],
			StartByte: Atoi(t[2]),
			EndByte:   Atoi(t[3]),
			Scope:     t[4],
			ScopeIdx:  AtoiInt(t[5]),
			Prefix:    byID[id],
		})
	}
	return fe
}
