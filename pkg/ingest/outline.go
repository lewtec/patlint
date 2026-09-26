package ingest

import (
	"slices"

	"github.com/lewtec/patlint/pkg/project"
)

// OutlineRow is one atoms-rail / browse line after scope nesting.
type OutlineRow struct {
	Depth     int
	Name      string
	Kind      string // "def" or "import"
	StartByte uint32
	EndByte   uint32
}

// OutlineFromExtract flattens FileExtract into a pre-order tree: each as-scope
// is labeled by its earliest own atom; other atoms and child scopes nest under it.
// Scopes with no descendant atoms are omitted. Imports sit at file depth.
func OutlineFromExtract(fe *project.FileExtract) []OutlineRow {
	if fe == nil {
		return nil
	}
	return OutlineFile(fe.Scopes, fe.Atoms, fe.Imports)
}

// OutlineFile nests atoms and imports by scope (store / Result slices).
func OutlineFile(scopes []project.ScopeDef, atoms []project.AtomDef, imports []project.ImportDef) []OutlineRow {
	n := len(scopes)
	atomsIn := make([][]int, n)
	var fileAtoms []int
	for i, a := range atoms {
		if a.Name == "" {
			continue
		}
		if a.ScopeIdx < 0 || a.ScopeIdx >= n {
			fileAtoms = append(fileAtoms, i)
			continue
		}
		atomsIn[a.ScopeIdx] = append(atomsIn[a.ScopeIdx], i)
	}
	children := make([][]int, n)
	var roots []int
	for i, s := range scopes {
		if s.Parent < 0 || s.Parent >= n {
			roots = append(roots, i)
			continue
		}
		children[s.Parent] = append(children[s.Parent], i)
	}
	hasAtoms := make([]bool, n)
	var mark func(int) bool
	mark = func(si int) bool {
		if si < 0 || si >= n {
			return false
		}
		if hasAtoms[si] {
			return true
		}
		if len(atomsIn[si]) > 0 {
			hasAtoms[si] = true
		}
		for _, c := range children[si] {
			if mark(c) {
				hasAtoms[si] = true
			}
		}
		return hasAtoms[si]
	}
	for i := range scopes {
		mark(i)
	}

	sortIdx := func(idxs []int, start func(int) uint32) {
		slices.SortStableFunc(idxs, func(a, b int) int {
			if start(a) != start(b) {
				if start(a) < start(b) {
					return -1
				}
				return 1
			}
			return a - b
		})
	}
	sortIdx(fileAtoms, func(i int) uint32 { return atoms[i].StartByte })
	for i := range atomsIn {
		sortIdx(atomsIn[i], func(j int) uint32 { return atoms[j].StartByte })
	}
	sortIdx(roots, func(i int) uint32 { return scopes[i].StartByte })
	for i := range children {
		sortIdx(children[i], func(j int) uint32 { return scopes[j].StartByte })
	}

	var out []OutlineRow
	var walkScope func(si, depth int)
	emitAtom := func(ai, depth int) {
		a := atoms[ai]
		out = append(out, OutlineRow{
			Depth:     depth,
			Name:      a.Name,
			Kind:      "def",
			StartByte: a.StartByte,
			EndByte:   a.EndByte,
		})
	}
	emitRest := func(si, depth, skip int) {
		type item struct {
			start uint32
			atom  int
			scope int
		}
		var items []item
		for _, ai := range atomsIn[si] {
			if ai == skip {
				continue
			}
			items = append(items, item{start: atoms[ai].StartByte, atom: ai, scope: -1})
		}
		for _, c := range children[si] {
			if !hasAtoms[c] {
				continue
			}
			items = append(items, item{start: scopes[c].StartByte, atom: -1, scope: c})
		}
		slices.SortStableFunc(items, func(a, b item) int {
			if a.start != b.start {
				if a.start < b.start {
					return -1
				}
				return 1
			}
			return 0
		})
		for _, it := range items {
			if it.atom >= 0 {
				emitAtom(it.atom, depth)
				continue
			}
			walkScope(it.scope, depth)
		}
	}
	walkScope = func(si, depth int) {
		if si < 0 || si >= n || !hasAtoms[si] {
			return
		}
		own := atomsIn[si]
		if len(own) == 0 {
			emitRest(si, depth, -1)
			return
		}
		header := own[0]
		emitAtom(header, depth)
		emitRest(si, depth+1, header)
	}

	type top struct {
		start uint32
		atom  int
		imp   int
		scope int
	}
	var tops []top
	for _, ai := range fileAtoms {
		tops = append(tops, top{start: atoms[ai].StartByte, atom: ai, imp: -1, scope: -1})
	}
	for i, im := range imports {
		name := im.LocalName
		if name == "" {
			name = LastPathComponent(im.SourcePath)
		}
		if name == "" {
			continue
		}
		tops = append(tops, top{start: im.StartByte, atom: -1, imp: i, scope: -1})
	}
	for _, si := range roots {
		if !hasAtoms[si] {
			continue
		}
		tops = append(tops, top{start: scopes[si].StartByte, atom: -1, imp: -1, scope: si})
	}
	slices.SortStableFunc(tops, func(a, b top) int {
		if a.start != b.start {
			if a.start < b.start {
				return -1
			}
			return 1
		}
		return 0
	})
	for _, t := range tops {
		switch {
		case t.atom >= 0:
			emitAtom(t.atom, 0)
		case t.imp >= 0:
			im := imports[t.imp]
			name := im.LocalName
			if name == "" {
				name = LastPathComponent(im.SourcePath)
			}
			out = append(out, OutlineRow{
				Depth:     0,
				Name:      name,
				Kind:      "import",
				StartByte: im.StartByte,
				EndByte:   im.EndByte,
			})
		default:
			walkScope(t.scope, 0)
		}
	}
	return out
}
