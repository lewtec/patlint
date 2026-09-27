package ingest

import (
	"github.com/lewtec/patlint/pkg/project"
	// MarkEntityRelationSpans returns per-file spans already covered by entity and
	// relation renames for the given source references.
)

func MarkEntityRelationSpans(result *project.Result, sourceSet StringSet) FileSpanSet {
	occupied := FileSpanSet{}
	if result == nil || sourceSet.Len() == 0 {
		return occupied
	}
	for _, ent := range result.Atoms {
		if !sourceSet.Has(ent.Reference) {
			continue
		}
		ref := ParseReference(ent.Reference)
		occupied.MarkRange(ref.Path, ent.StartByte, ent.EndByte)
	}
	for _, rel := range result.Uses {
		if !sourceSet.Has(rel.Target) {
			continue
		}
		ref := ParseReference(rel.Reference)
		occupied.MarkRange(ref.Path, rel.StartByte, rel.EndByte)
	}
	return occupied
}
