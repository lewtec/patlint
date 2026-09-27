package pattern

import (
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// siteEditMaskSpans returns body spans of site edits for prune masking.
// Insert-only edits (Start==End) are skipped.
func SiteEditMaskSpans(edits []project.Edit) []ingestutil.Span {
	if len(edits) == 0 {
		return nil
	}
	ss := make([]ingestutil.Span, len(edits))
	for i, e := range edits {
		ss[i] = e.Span
	}
	return ingestutil.NonEmpty(ss)
}

// ImportNeedsForRule collects language import needs from static refs in the
// replacement. Unknown languages or refs yield no needs.
func ImportNeedsForRule(policy ingest.PackQueries, lang string, r Rule) []ingest.ImportNeed {
	if policy == nil {
		return nil
	}
	seen := map[string]bool{}
	var needs []ingest.ImportNeed
	for _, ref := range EmitRefs(r.Emit) {
		n, ok := policy.ImportNeedFromRef(lang, ref)
		if !ok || n.ImportPath == "" || seen[n.ImportPath] {
			continue
		}
		seen[n.ImportPath] = true
		needs = append(needs, n)
	}
	return needs
}

// WithImportHygiene appends ensure + named prune edits onto siteEdits.
// Import edits are computed on the original source so their offsets stay in
// the preamble; ApplyEdits (high offsets first) applies body sites before the
// import region. prune.MaskSpans should be rewrite match spans (or empty).
// Empty OnlyCandidates prunes any named import unused after masking.
func WithImportHygiene(policy ingest.PackQueries, fileRel, lang string, source []byte, fe *project.FileExtract, siteEdits []project.Edit, needs []ingest.ImportNeed, prune ingest.PruneImportOpts) []project.Edit {
	if len(siteEdits) == 0 {
		return siteEdits
	}
	out := append([]project.Edit(nil), siteEdits...)
	if len(needs) > 0 {
		imp := ingest.EnsureImportAfterLastMarked(fileRel, source, fe, needs)
		if len(imp) == 0 {
			imp = ingest.EnsureImportFirst(fileRel, source, fe, policy.ImportLineInfo(lang), policy, needs)
		}
		out = append(out, imp...)
	}
	out = append(out, ingest.PruneNamedUnusedFromExtract(fileRel, source, fe, prune)...)
	return out
}
