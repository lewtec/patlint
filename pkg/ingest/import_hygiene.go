package ingest

import (
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// ImportNeed is one import the file should have after a structural edit.
// Interpretation is language-specific (Go: ImportPath is the quoted path).
type ImportNeed struct {
	ImportPath string
}

// PruneImportOpts controls named-only unused-import removal.
//
// MaskSpans are body regions treated as deleted for usage (moved decl, rewrite
// match spans). OnlyCandidates, when non-empty, limits prune to those import
// keys (language-specific: Go import path; JS/Java full import statement text
// as in DeclExtract.Imports). Empty OnlyCandidates means all named imports.
//
// Never removes barrel-style imports (side-effect, star/namespace, Go blank/dot).
type PruneImportOpts struct {
	MaskSpans      []ingestutil.Span
	OnlyCandidates []string
}

// PruneNamedUnusedForDecl is the shared residual path for cross-file moves:
// prune named imports that only the removed declaration needed.
// Empty decl.Imports means no candidates (do not full-file prune).
func PruneNamedUnusedForDecl(fileRel string, content []byte, fe *project.FileExtract, decl DeclExtract) []project.Edit {
	if fe == nil || len(content) == 0 || len(decl.Imports) == 0 {
		return nil
	}
	return PruneNamedUnusedFromExtract(fileRel, content, fe, PruneImportOpts{
		MaskSpans: []ingestutil.Span{{
			StartByte: decl.RemoveStart,
			EndByte:   decl.RemoveEnd,
		}},
		OnlyCandidates: decl.Imports,
	})
}
