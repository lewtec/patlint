package ingest

import (
	"github.com/lewtec/patlint/pkg/project"
	// MethodRenameScope is the shared ExtraRename partition: which atoms are being
	// renamed, which type/class receivers we own, and which same-leaf receivers are
	// foreign (must not rewrite their call sites).
)

type MethodRenameScope struct {
	Sources StringSet // full atom references (path:…::Name)
	Our     StringSet // receivers belonging to the rename (Class / Type)
	Foreign StringSet // other types that also define oldLeaf
	OldLeaf string
	NewLeaf string
}

// AtomReceiverFunc extracts the type/class prefix from an atom name
// (e.g. "Box.get" → "Box"). ok=false for free functions.
type AtomReceiverFunc func(atomName string) (receiver string, ok bool)

// SkipForeignFunc, if non-nil, returns true to not mark a same-leaf atom as
// foreign (e.g. Go interface types co-renamed via another path).
type SkipForeignFunc func(ent project.Atom, recv string) bool

// PartitionMethodRename builds Sources/Our from sourceRefs and Foreign from
// other atoms with the same leaf. Does not apply language hierarchy expansion
// (implements/bases); callers may Add into Our/Foreign afterward.
func PartitionMethodRename(result *project.Result, sourceRefs []string, oldLeaf, newLeaf string, recv AtomReceiverFunc, skipForeign SkipForeignFunc) MethodRenameScope {
	scope := MethodRenameScope{
		Sources: NewStringSet(),
		Our:     NewStringSet(),
		Foreign: NewStringSet(),
		OldLeaf: oldLeaf,
		NewLeaf: newLeaf,
	}
	if recv == nil {
		return scope
	}
	for _, s := range sourceRefs {
		if s == "" {
			continue
		}
		scope.Sources.Add(s)
		ref := ParseReference(s)
		if r, ok := recv(ref.Name); ok {
			scope.Our.Add(r)
		}
	}
	if result == nil || oldLeaf == "" {
		return scope
	}
	for _, ent := range result.Atoms {
		if scope.Sources.Has(ent.Reference) {
			continue
		}
		ref := ParseReference(ent.Reference)
		if AtomName(ref.Name) != oldLeaf {
			continue
		}
		r, ok := recv(ref.Name)
		if !ok || scope.Our.Has(r) {
			continue
		}
		if skipForeign != nil && skipForeign(ent, r) {
			continue
		}
		scope.Foreign.Add(r)
	}
	return scope
}

// OccupiedSpans returns FileSpanSet for entity/use spans of Sources.
func (m MethodRenameScope) OccupiedSpans(result *project.Result) FileSpanSet {
	return MarkEntityRelationSpans(result, m.Sources)
}
