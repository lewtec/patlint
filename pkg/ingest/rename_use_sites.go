package ingest

import (
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"log/slog"
	"sync"
)

// UseSiteRenamer expands leaf renames at call/use sites for symbols in
// sourceSet. newLeaf is the replacement identifier text (AtomName form).
// root is the project ingest root; result is a full project materialize.
//
// The default walks result.Uses (graph). Linking pkg/pattern registers
// pattern.UseSiteRenames (RefLeafRule / NFA) so mv use sites share the same
// site-transform backbone as rewrite.
type UseSiteRenamer func(root string, result *project.Result, sourceSet StringSet, newLeaf string) []project.Edit

var (
	useSiteRenamerMu sync.RWMutex
	useSiteRenamer   UseSiteRenamer
)

// RegisterUseSiteRenamer sets the use-site rename expander.
// Passing nil restores the graph default. Panics if called twice with non-nil
// without an intervening nil clear (same idea as one global site engine).
func RegisterUseSiteRenamer(fn UseSiteRenamer) {
	useSiteRenamerMu.Lock()
	defer useSiteRenamerMu.Unlock()
	useSiteRenamer = fn
}

func expandUseSiteRenames(root string, result *project.Result, sourceSet StringSet, newLeaf string) []project.Edit {
	useSiteRenamerMu.RLock()
	fn := useSiteRenamer
	useSiteRenamerMu.RUnlock()
	if fn != nil {
		edits := fn(root, result, sourceSet, newLeaf)
		slog.Debug("use-site renames: rule expander", "edits", len(edits), "new_leaf", newLeaf)
		return edits
	}
	edits := useSiteRenamesFromGraph(result, sourceSet, newLeaf)
	slog.Debug("use-site renames: graph fallback", "edits", len(edits), "new_leaf", newLeaf)
	return edits
}

// useSiteRenamesFromGraph rewrites identifier leaves at Uses that target any
// ref in sourceSet. Import-alias bindings (ViaImportAlias) are left unchanged.
func useSiteRenamesFromGraph(result *project.Result, sourceSet StringSet, newLeaf string) []project.Edit {
	var edits []project.Edit
	for _, rel := range result.Uses {
		if !sourceSet.Has(rel.Target) || rel.ViaImportAlias {
			continue
		}
		ref := ParseReference(rel.Reference)
		edits = AppendReplaceSpan(edits, ref.Path, ingestutil.Span{StartByte: rel.StartByte, EndByte: rel.EndByte}, newLeaf)
	}
	return edits
}
