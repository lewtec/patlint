package ingest

import (
	"sort"
	"strings"

	"github.com/lewtec/patlint/pkg/project"
)

// refCache is AtomRef/FileRef memo for scope tests and leftover helpers.
type refCache struct {
	atoms map[string]map[string]string
	files map[string]string
	dots  map[string]string
}

func (c *refCache) Atom(path, name string) string {
	if name == "" {
		return c.File(path)
	}
	m := c.atoms[path]
	if m == nil {
		if c.atoms == nil {
			c.atoms = make(map[string]map[string]string)
		}
		m = make(map[string]string)
		c.atoms[path] = m
	}
	if s, ok := m[name]; ok {
		return s
	}
	s := AtomRef(path, name)
	m[name] = s
	return s
}

func (c *refCache) File(path string) string {
	if c.files == nil {
		c.files = make(map[string]string)
	}
	if s, ok := c.files[path]; ok {
		return s
	}
	s := FileRef(path)
	c.files[path] = s
	return s
}

func (c *refCache) Path(rel string) string {
	if c.dots == nil {
		c.dots = make(map[string]string)
	}
	if s, ok := c.dots[rel]; ok {
		return s
	}
	s := rel
	if !strings.HasPrefix(rel, "./") && !strings.HasPrefix(rel, "../") && !strings.HasPrefix(rel, "/") {
		s = "./" + rel
	}
	c.dots[rel] = s
	return s
}

// scopeTree is one file's scopes: local symbol maps + a cut list for At.
// Lookup walks Parent. Inner maps shadow outer. No types.
type scopeTree struct {
	parent []int               // per as-scope; -1 = file
	syms   []map[string]string // local decls only; may be nil
	file   map[string]string   // file root (idx -1)
	cuts   []scopeCut
	pkg    string
	lang   string
	path   string
}

type scopeCut struct {
	off uint32
	idx int
}

type scopeEvent struct {
	off   uint32
	end   uint32
	idx   int
	start bool
}

func buildScopeTree(fe *project.FileExtract, rc *refCache) *scopeTree {
	t := &scopeTree{file: map[string]string{}}
	if fe == nil {
		return t
	}
	t.pkg = fe.Package
	t.lang = fe.Language
	t.path = fe.Path
	n := len(fe.Scopes)
	t.parent = make([]int, n)
	t.syms = make([]map[string]string, n)
	for i, s := range fe.Scopes {
		t.parent[i] = s.Parent
	}
	t.cuts = buildScopeCuts(fe.Scopes)
	fp := ""
	if rc != nil {
		fp = rc.Path(fe.Path)
	}
	for _, a := range fe.Atoms {
		leaf := AtomName(a.Name)
		if leaf == "" {
			continue
		}
		idx := declareScope(fe, a)
		if idx < -1 {
			continue
		}
		ref := a.Name
		if rc != nil {
			ref = rc.Atom(fp, a.Name)
		}
		if idx < 0 {
			if _, ok := t.file[leaf]; !ok {
				t.file[leaf] = ref
			}
			continue
		}
		if t.syms[idx] == nil {
			t.syms[idx] = map[string]string{}
		}
		if _, ok := t.syms[idx][leaf]; !ok {
			t.syms[idx][leaf] = ref
		}
	}
	return t
}

// declareScope is the scope that owns atom's leaf.
// Type.leaf (except Type.Type) that is the first atom in its scope (the
// type/method itself) → enclosing type. Later dotted atoms (Type.method.local)
// stay in the method so same-leaf params in sibling methods do not collide.
// Type.Type stays off the type table so the type name is not shadowed by a
// same-leaf method/ctor. The first atom in a scope is that scope's name and
// lives on the parent (type/func → file or namespace). Later bare atoms are locals.
func declareScope(fe *project.FileExtract, a project.AtomDef) int {
	inn := a.ScopeIdx
	if i := strings.LastIndex(a.Name, "."); i > 0 {
		typ, leaf := a.Name[:i], a.Name[i+1:]
		if leaf == typ || strings.HasSuffix(typ, "."+leaf) {
			return -2
		}
		if inn < 0 || inn >= len(fe.Scopes) {
			return -1
		}
		if !firstAtomInScope(fe, inn, a) {
			return inn
		}
		if p := fe.Scopes[inn].Parent; p >= 0 {
			return p
		}
		return inn
	}
	if inn < 0 || inn >= len(fe.Scopes) {
		return -1
	}
	if firstAtomInScope(fe, inn, a) {
		return fe.Scopes[inn].Parent
	}
	return inn
}

func firstAtomInScope(fe *project.FileExtract, scopeIdx int, a project.AtomDef) bool {
	for _, o := range fe.Atoms {
		if o.ScopeIdx != scopeIdx {
			continue
		}
		if o.StartByte < a.StartByte {
			return false
		}
	}
	return true
}

func buildScopeCuts(scopes []project.ScopeDef) []scopeCut {
	if len(scopes) == 0 {
		return nil
	}
	evs := make([]scopeEvent, 0, len(scopes)*2)
	for i, s := range scopes {
		if s.EndByte <= s.StartByte {
			continue
		}
		evs = append(evs,
			scopeEvent{off: s.StartByte, end: s.EndByte, idx: i, start: true},
			scopeEvent{off: s.EndByte, end: s.EndByte, idx: i, start: false},
		)
	}
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].off != evs[j].off {
			return evs[i].off < evs[j].off
		}
		if evs[i].start != evs[j].start {
			return !evs[i].start // ends before starts at the same byte
		}
		if evs[i].start {
			return evs[i].end > evs[j].end // outer start first
		}
		return evs[i].end < evs[j].end // inner end first
	})
	stack := []int{-1}
	cuts := make([]scopeCut, 0, len(evs)+1)
	cuts = append(cuts, scopeCut{off: 0, idx: -1})
	for _, ev := range evs {
		if ev.start {
			stack = append(stack, ev.idx)
		} else if len(stack) > 1 {
			stack = stack[:len(stack)-1]
		}
		top := stack[len(stack)-1]
		if n := len(cuts); n > 0 && cuts[n-1].off == ev.off {
			cuts[n-1].idx = top
			continue
		}
		cuts = append(cuts, scopeCut{off: ev.off, idx: top})
	}
	return cuts
}

// At returns the innermost as-scope index containing off, or -1 for the file.
func (t *scopeTree) At(off uint32) int {
	if t == nil || len(t.cuts) == 0 {
		return -1
	}
	i, j := 0, len(t.cuts)
	for i < j {
		m := (i + j) / 2
		if t.cuts[m].off <= off {
			i = m + 1
		} else {
			j = m
		}
	}
	if i == 0 {
		return -1
	}
	return t.cuts[i-1].idx
}

// Lookup resolves name at off by walking from At(off) to the file.
func (t *scopeTree) Lookup(off uint32, name string) string {
	if t == nil || name == "" {
		return ""
	}
	for i := t.At(off); ; {
		if i < 0 {
			return t.file[name]
		}
		if m := t.syms[i]; m != nil {
			if ref, ok := m[name]; ok {
				return ref
			}
		}
		if i >= len(t.parent) {
			return t.file[name]
		}
		i = t.parent[i]
	}
}
