package ingest

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// ImportBinding is the lowest-common-denominator form of one import name binding.
// Languages parse their own syntax into this shape; shared algorithms decide
// which bindings a declaration needs (DeclExtract.Imports / residual prune keys).
//
// Key is language-specific identity for OnlyCandidates (Go: import path or
// "alias path"; JS/Java: full statement text). Local is the residual-use name.
type ImportBinding struct {
	Local string
	Key   string
	// KeepAlways forces the Key into NeededImportKeys even when Local is not a
	// normal ident (e.g. Java on-demand import ".*").
	KeepAlways bool
}

// ImportSite is one prunable import region in a file (LCD of go/js/java prune).
// Locals are names that keep the site alive when used in residual body text.
// KeepAlways means never prune (barrel / blank / star / side-effect).
type ImportSite struct {
	Key        string
	Locals     []string
	Span       ingestutil.Span
	KeepAlways bool
}

// CandidateKeySet builds an OnlyCandidates lookup map. Empty only → nil (all keys).
func CandidateKeySet(only []string) map[string]bool {
	if len(only) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, s := range only {
		s = strings.TrimSpace(s)
		if s != "" {
			want[s] = true
		}
	}
	if len(want) == 0 {
		return nil
	}
	return want
}

// IdentUsage reports whether local appears as a usable identifier in text.
// Languages pass comment-aware checkers (Go) or IdentUsed with a char class.
type IdentUsage func(text, local string) bool

// NeededImportKeys returns Key for each binding that declText needs.
// Dedupes by Key, preserves first-seen order. Nil used defaults to ASCII IdentUsed.
func NeededImportKeys(declText string, bindings []ImportBinding, used IdentUsage) []string {
	if used == nil {
		used = func(text, local string) bool {
			return ingestutil.IdentUsed(text, local, ingestutil.IsIdentChar)
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, b := range bindings {
		if b.Key == "" || seen[b.Key] {
			continue
		}
		if b.KeepAlways {
			seen[b.Key] = true
			out = append(out, b.Key)
			continue
		}
		if b.Local == "" || b.Local == "*" || b.Local == "." || b.Local == "_" {
			continue
		}
		if used(declText, b.Local) {
			seen[b.Key] = true
			out = append(out, b.Key)
		}
	}
	return out
}

// PruneUnusedImportSites removes sites whose Locals are unused after masking
// opts.MaskSpans and every site span. Matches opts.OnlyCandidates against Key
// when set. Never prunes KeepAlways sites or sites with no Locals when not KeepAlways
// (treat empty Locals + !KeepAlways as non-named / skip — caller should mark barrels KeepAlways).
func PruneUnusedImportSites(fileRel string, content []byte, opts PruneImportOpts, sites []ImportSite, used IdentUsage) []project.Edit {
	if len(content) == 0 || len(sites) == 0 {
		return nil
	}
	if used == nil {
		used = func(text, local string) bool {
			return ingestutil.IdentUsed(text, local, ingestutil.IsIdentChar)
		}
	}
	want := CandidateKeySet(opts.OnlyCandidates)
	if len(opts.OnlyCandidates) > 0 && want == nil {
		return nil
	}

	masked := append([]byte(nil), content...)
	for _, sp := range opts.MaskSpans {
		ingestutil.MaskNonNewlinesInPlace(masked, int(sp.StartByte), int(sp.EndByte))
	}
	for _, site := range sites {
		ingestutil.MaskNonNewlinesInPlace(masked, int(site.Span.StartByte), int(site.Span.EndByte))
	}
	rest := string(masked)

	var edits []project.Edit
	for _, site := range sites {
		if site.KeepAlways {
			continue
		}
		if want != nil && !want[site.Key] {
			continue
		}
		if len(site.Locals) == 0 {
			continue
		}
		stillUsed := false
		for _, local := range site.Locals {
			if local == "" || local == "*" || local == "." || local == "_" {
				continue
			}
			if used(rest, local) {
				stillUsed = true
				break
			}
		}
		if stillUsed {
			continue
		}
		edits = append(edits, project.Edit{
			File:    fileRel,
			Span:    site.Span,
			NewText: "",
		})
	}
	return edits
}

// AppendUnoccupied appends e when its span is free in occ, and marks the span.
// A nil occ always accepts the edit (and does not record).
func AppendUnoccupied(edits []project.Edit, occ *SpanSet, e project.Edit) []project.Edit {
	if occ != nil {
		if occ.Overlaps(e.Span) {
			return edits
		}
		occ.Add(e.Span)
	}
	return append(edits, e)
}

// AppendUnoccupiedAll appends each candidate that does not overlap occ.
func AppendUnoccupiedAll(edits []project.Edit, occ *SpanSet, candidates []project.Edit) []project.Edit {
	for _, e := range candidates {
		edits = AppendUnoccupied(edits, occ, e)
	}
	return edits
}
