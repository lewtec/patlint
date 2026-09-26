package ingest

import (
	"fmt"
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// BindingSite is one occurrence of a local (or parameter) name: either the
// defining span or a use of that binding. DefSpan always points at the def.
type BindingSite struct {
	// Key is opaque and stable within one file's BindingIndex (not a product ref).
	Key string
	// Name is the surface identifier text.
	Name string
	// DefSpan is the defining occurrence (parameters, := left, var name, …).
	DefSpan ingestutil.Span
	// IsDef is true when this map entry's key span is the definition site.
	IsDef bool
}

// BindingIndex maps every def/use span to its binding for O(1) annotate lookup.
// ingestutil.Span keys are exact leaf spans (tree-sitter identifier nodes).
type BindingIndex struct {
	BySpan map[ingestutil.Span]BindingSite
}

// NewBindingIndex returns an empty index.
func NewBindingIndex() *BindingIndex {
	return &BindingIndex{BySpan: map[ingestutil.Span]BindingSite{}}
}

// Put records a site at span. An existing def at the same span is kept.
func (idx *BindingIndex) Put(span ingestutil.Span, site BindingSite) {
	if idx == nil || span.Empty() {
		return
	}
	if idx.BySpan == nil {
		idx.BySpan = map[ingestutil.Span]BindingSite{}
	}
	if old, ok := idx.BySpan[span]; ok && old.IsDef {
		return
	}
	idx.BySpan[span] = site
}

// Lookup returns the site for span, if any.
func (idx *BindingIndex) Lookup(span ingestutil.Span) (BindingSite, bool) {
	if idx == nil || idx.BySpan == nil {
		return BindingSite{}, false
	}
	s, ok := idx.BySpan[span]
	return s, ok
}

// BindingIndexFromExtract maps pack atoms (defs) and uses in the same as-scope
// chain. Product refs win at annotate time; this is only local/param holes.
func BindingIndexFromExtract(fe *project.FileExtract) *BindingIndex {
	idx := NewBindingIndex()
	if fe == nil {
		return idx
	}
	type localDef struct {
		name string
		span ingestutil.Span
		key  string
	}
	byScope := map[int][]localDef{}
	for _, a := range fe.Atoms {
		name := atomLeafName(a.Name)
		if name == "" {
			continue
		}
		sp := ingestutil.Span{StartByte: a.StartByte, EndByte: a.EndByte}
		if sp.Empty() {
			continue
		}
		key := fmt.Sprintf("%s@%d", name, a.StartByte)
		byScope[a.ScopeIdx] = append(byScope[a.ScopeIdx], localDef{name: name, span: sp, key: key})
		idx.Put(sp, BindingSite{Key: key, Name: name, DefSpan: sp, IsDef: true})
	}
	lookup := func(name string, scopeIdx int) (localDef, bool) {
		for {
			for _, d := range byScope[scopeIdx] {
				if d.name == name {
					return d, true
				}
			}
			if scopeIdx < 0 || scopeIdx >= len(fe.Scopes) {
				return localDef{}, false
			}
			scopeIdx = fe.Scopes[scopeIdx].Parent
		}
	}
	for _, u := range fe.Usages {
		name := atomLeafName(u.Name)
		if name == "" {
			continue
		}
		sp := ingestutil.Span{StartByte: u.StartByte, EndByte: u.EndByte}
		if sp.Empty() {
			continue
		}
		d, ok := lookup(name, u.ScopeIdx)
		if ok {
			if sp.StartByte == d.span.StartByte && sp.EndByte == d.span.EndByte {
				continue
			}
			idx.Put(sp, BindingSite{Key: d.key, Name: name, DefSpan: d.span, IsDef: false})
			continue
		}
		// Bare first mention in this scope is the def (params / range vars
		// that pack extract did not emit as atoms).
		if len(u.Prefix) > 0 {
			continue
		}
		key := fmt.Sprintf("%s@%d", name, u.StartByte)
		nd := localDef{name: name, span: sp, key: key}
		byScope[u.ScopeIdx] = append(byScope[u.ScopeIdx], nd)
		idx.Put(sp, BindingSite{Key: key, Name: name, DefSpan: sp, IsDef: true})
	}
	return idx
}

func atomLeafName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}
