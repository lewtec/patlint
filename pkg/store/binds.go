package store

import (
	"sort"
	"strings"

	"github.com/lewtec/patlint/pkg/reference"
)

// FillBinds is stratum 2 bind walk. Datalog already closed alias / visible /
// import_target. One pass per use, priority matches the old ladder.
func FillBinds(s *Store) {
	if s == nil {
		return
	}
	imp := indexImportTargets(s)
	vis := indexVisible(s)
	atomsAt := indexAtomsBySpan(s)
	fileMeta := indexFiles(s)
	decl := indexDeclares(s)
	atomIn := indexAtomInFile(s)
	pkgPeers := indexPkgPeers(s, fileMeta, decl)
	includes := indexIncludes(s, fileMeta, atomIn)
	stars := indexStars(s)

	type useKey struct{ f, id string }
	segs := map[useKey][]Tuple{}
	for _, t := range s.Rows(RelationUseSegment) {
		if len(t) < 6 {
			continue
		}
		k := useKey{t[0], t[1]}
		segs[k] = append(segs[k], t)
	}
	for k, sl := range segs {
		sort.Slice(sl, func(i, j int) bool { return AtoiInt(sl[i][2]) < AtoiInt(sl[j][2]) })
		segs[k] = sl
	}

	for _, u := range s.Rows(RelationUse) {
		if len(u) < 7 {
			continue
		}
		f, name, start, end, scope, si, npre := u[0], u[1], u[2], u[3], u[4], AtoiInt(u[5]), AtoiInt(u[6])
		id := "0"
		if len(u) > 7 {
			id = u[7]
		}
		from := useFrom(s, f, scope)
		if npre > 0 {
			fillPrefix(s, f, start, end, name, from, segs[useKey{f, id}], imp[f], vis, si, atomIn, pkgPeers)
			continue
		}
		target, via := "", "0"
		if it, ok := imp[f][name]; ok {
			target, via = it.target, it.alias
		}
		if target == "" {
			if t, ok := atomsAt[spanKey{f, start, end}]; ok && (atomLeaf(t.name) == name || t.name == name) {
				target = t.ref
			}
		}
		if target == "" {
			target, via = resolveBare(f, name, si, scope, nil, vis, pkgPeers, includes, fileMeta[f], atomIn)
			if via == "" {
				via = "0"
			}
		}
		if target == "" && !strings.HasPrefix(name, "_") {
			target = stars.lookup(f, name, atomIn)
		}
		if target == "" {
			continue
		}
		s.Append(RelationBinds, Tuple{f, start, end, from, target, via})
	}
}

type impT struct{ target, alias, mem string }

func indexImportTargets(s *Store) map[string]map[string]impT {
	out := map[string]map[string]impT{}
	for _, t := range s.Rows(RelationImportTarget) {
		if len(t) < 5 {
			continue
		}
		m := out[t[0]]
		if m == nil {
			m = map[string]impT{}
			out[t[0]] = m
		}
		m[t[1]] = impT{target: t[2], mem: t[3], alias: t[4]}
	}
	return out
}

type visIdx map[string]map[int]map[string]string

func indexVisible(s *Store) visIdx {
	out := visIdx{}
	for _, t := range s.Rows(RelationVisible) {
		if len(t) < 4 {
			continue
		}
		f := t[0]
		si := AtoiInt(t[1])
		if out[f] == nil {
			out[f] = map[int]map[string]string{}
		}
		if out[f][si] == nil {
			out[f][si] = map[string]string{}
		}
		if _, ok := out[f][si][t[2]]; !ok {
			out[f][si][t[2]] = t[3]
		}
	}
	return out
}

func (v visIdx) lookup(file string, si int, name string) string {
	for i := si; ; {
		if m := v[file][i]; m != nil {
			if t, ok := m[name]; ok {
				return t
			}
		}
		if i < 0 {
			return ""
		}
		// parent is on RelationScope; walk via -1 after first miss at i
		if i == -1 {
			return ""
		}
		// fall to file root
		i = -1
	}
}

type spanKey struct{ f, s, e string }

type atomSpan struct{ name, ref string }

func indexAtomsBySpan(s *Store) map[spanKey]atomSpan {
	out := map[spanKey]atomSpan{}
	for _, t := range s.Rows(RelationAtom) {
		if len(t) < 6 {
			continue
		}
		out[spanKey{t[0], t[2], t[3]}] = atomSpan{name: t[1], ref: reference.AtomRef(pathDot(t[0]), t[1])}
	}
	return out
}

type fileInfo struct {
	lang, pkg string
	nested    bool
	incl      bool
	pkgScope  bool
	emptyDir  bool
}

func indexFiles(s *Store) map[string]fileInfo {
	out := map[string]fileInfo{}
	flags := map[string]map[string]bool{}
	for _, t := range s.Rows(RelationLanguage) {
		if len(t) < 2 {
			continue
		}
		if flags[t[0]] == nil {
			flags[t[0]] = map[string]bool{}
		}
		flags[t[0]][t[1]] = true
	}
	for _, t := range s.Rows(RelationFile) {
		if len(t) < 3 {
			continue
		}
		fl := flags[t[1]]
		out[t[0]] = fileInfo{
			lang: t[1], pkg: t[2],
			nested:   fl[FlagNested],
			incl:     fl[FlagIncludeExport],
			pkgScope: fl[FlagPackageScoped],
			emptyDir: fl[FlagEmptyDir],
		}
	}
	return out
}

type peerIdx struct {
	byPkg map[string][]string
	meta  map[string]fileInfo
	decl  declIdx
}

type declIdx map[string]map[string]string // file → leaf → ref (file-level)

func indexDeclares(s *Store) declIdx {
	out := declIdx{}
	for _, t := range s.Rows(RelationDeclares) {
		if len(t) < 4 || t[1] != "-1" {
			continue
		}
		if out[t[0]] == nil {
			out[t[0]] = map[string]string{}
		}
		if _, ok := out[t[0]][t[2]]; !ok {
			out[t[0]][t[2]] = t[3]
		}
	}
	return out
}

func indexPkgPeers(_ *Store, meta map[string]fileInfo, decl declIdx) peerIdx {
	p := peerIdx{byPkg: map[string][]string{}, meta: meta, decl: decl}
	for f, fi := range meta {
		if !fi.pkgScope {
			continue
		}
		k := fi.lang + "\x00" + fi.pkg
		p.byPkg[k] = append(p.byPkg[k], f)
	}
	return p
}

func (p peerIdx) lookup(file, name string) string {
	fi := p.meta[file]
	if !fi.pkgScope {
		return ""
	}
	for _, g := range p.byPkg[fi.lang+"\x00"+fi.pkg] {
		if g == file {
			continue
		}
		if fi.emptyDir && fi.pkg == "" && dirOf(g) != dirOf(file) {
			continue
		}
		if t, ok := p.decl[g][name]; ok {
			return t
		}
	}
	return ""
}

type inclIdx struct {
	files map[string][]string
	atoms atomInIdx
}

func indexIncludes(s *Store, meta map[string]fileInfo, atoms atomInIdx) inclIdx {
	out := inclIdx{files: map[string][]string{}, atoms: atoms}
	for f, fi := range meta {
		if !fi.incl {
			continue
		}
		for _, t := range s.Rows(RelationImportTarget) {
			if len(t) < 3 || t[0] != f {
				continue
			}
			r := reference.Parse(t[2])
			if r.Name != "" || (r.Provider != "path" && r.Provider != "") {
				continue
			}
			out.files[f] = append(out.files[f], pathDot(r.Path))
		}
	}
	return out
}

func (i inclIdx) lookup(file, name string) string {
	for _, inc := range i.files[file] {
		if t, ok := i.atoms.lookup(inc, name); ok {
			return t
		}
	}
	return ""
}

func (i inclIdx) lookupNested(file, qual string) string {
	for _, inc := range i.files[file] {
		if t, ok := i.atoms.lookup(inc, qual); ok {
			return t
		}
	}
	return ""
}

type starIdx struct {
	bases map[string][]string
}

func indexStars(s *Store) starIdx {
	out := starIdx{bases: map[string][]string{}}
	for _, t := range s.Rows(RelationStarBase) {
		if len(t) < 2 {
			continue
		}
		out.bases[t[0]] = append(out.bases[t[0]], t[1])
	}
	return out
}

func (st starIdx) lookup(file, name string, atoms atomInIdx) string {
	bases := st.bases[file]
	for i := len(bases) - 1; i >= 0; i-- {
		r := reference.Parse(bases[i])
		if r.Name != "" {
			continue
		}
		if t, ok := atoms.lookup(pathDot(r.Path), name); ok {
			return t
		}
	}
	return ""
}

type atomInIdx map[string]map[string]string

func indexAtomInFile(s *Store) atomInIdx {
	out := atomInIdx{}
	for _, t := range s.Rows(RelationAtomInFile) {
		if len(t) < 3 {
			continue
		}
		if out[t[0]] == nil {
			out[t[0]] = map[string]string{}
		}
		if _, ok := out[t[0]][t[1]]; !ok {
			out[t[0]][t[1]] = t[2]
		}
	}
	return out
}

func (a atomInIdx) lookup(file, name string) (string, bool) {
	for _, k := range []string{file, pathDot(file), strings.TrimPrefix(file, "./")} {
		if t, ok := a[k][name]; ok {
			return t, true
		}
	}
	return "", false
}

func useFrom(s *Store, file, scope string) string {
	if scope == "" {
		return reference.FileRef(pathDot(file))
	}
	var found string
	n := 0
	for _, t := range s.Rows(RelationAtom) {
		if len(t) < 2 || t[0] != file {
			continue
		}
		if t[1] == scope || atomLeaf(t[1]) == scope {
			n++
			found = t[1]
		}
	}
	if n == 1 {
		return reference.AtomRef(pathDot(file), found)
	}
	if n > 1 {
		for _, t := range s.Rows(RelationAtom) {
			if len(t) >= 2 && t[0] == file && t[1] == scope {
				return reference.AtomRef(pathDot(file), scope)
			}
		}
	}
	return reference.AtomRef(pathDot(file), scope)
}

func resolveBare(f, name string, si int, scope string, imp map[string]impT, vis visIdx, pkgPeers peerIdx, includes inclIdx, fi fileInfo, atoms atomInIdx) (target, via string) {
	if it, ok := imp[name]; ok {
		return it.target, it.alias
	}
	if t := vis.lookup(f, si, name); t != "" {
		return t, "0"
	}
	if t := pkgPeers.lookup(f, name); t != "" {
		return t, "0"
	}
	if t := includes.lookup(f, name); t != "" {
		return t, "0"
	}
	if scope != "" {
		if t, ok := atoms.lookup(f, scope+"."+name); ok {
			return t, "0"
		}
		if fi.nested {
			head := scope
			if i := strings.Index(head, "."); i >= 0 {
				head = head[:i]
			}
			if t, ok := atoms.lookup(f, head+"."+name); ok {
				return t, "0"
			}
			if t := includes.lookupNested(f, head+"."+name); t != "" {
				return t, "0"
			}
		}
	}
	return "", "0"
}

func fillPrefix(s *Store, f, leafS, leafE, leaf, from string, segs []Tuple, imp map[string]impT, vis visIdx, si int, atoms atomInIdx, pkgPeers peerIdx) {
	var base, importMem string
	for _, seg := range segs {
		idx := AtoiInt(seg[2])
		name, ss, se := seg[3], seg[4], seg[5]
		var target string
		if idx == 0 {
			if it, ok := imp[name]; ok {
				target = it.target
				importMem = it.mem
				base = target
				if it.mem != "" {
					base = strings.TrimSuffix(it.target, "::"+it.mem)
				}
			} else {
				target = vis.lookup(f, si, name)
				if target == "" {
					target = pkgPeers.lookup(f, name)
				}
				base = target
			}
		} else {
			target = hop(atoms, base, importMem, name)
			base = target
			importMem = ""
		}
		if target == "" {
			return
		}
		s.Append(RelationBinds, Tuple{f, ss, se, from, target, "0"})
	}
	if base == "" {
		return
	}
	s.Append(RelationBinds, Tuple{f, leafS, leafE, from, hop(atoms, base, importMem, leaf), "0"})
}

func hop(atoms atomInIdx, base, importMem, member string) string {
	if member == "" || base == "" {
		return ""
	}
	r := reference.Parse(base)
	file := strings.TrimPrefix(r.Path, "./")
	if r.Name != "" && (r.Provider == "path" || r.Provider == "") {
		if t, ok := atoms.lookup(file, r.Name+"."+member); ok {
			return t
		}
		return base + "::" + member
	}
	if importMem != "" {
		if t, ok := atoms.lookup(file, importMem+"."+member); ok {
			return t
		}
		if t, ok := atoms.lookup(file, member); ok {
			return t
		}
		return base + "::" + member
	}
	return base + "::" + member
}
