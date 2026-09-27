package ingest

import "github.com/lewtec/patlint/pkg/ingestutil"

import "strings"

// SpanSet is a set of half-open Spans in one buffer (typically one file).
// Used to avoid double-rewriting the same sites during rename/extra edits.
// A nil *SpanSet is empty: Overlaps is always false; Add panics on nil receiver
// (use NewSpanSet or FileSpanSet.For).
type SpanSet struct {
	// m stores exact span keys; Overlaps also reports any intersecting span.
	m map[[2]uint32]struct{}
}

// NewSpanSet returns an empty set.
func NewSpanSet() *SpanSet {
	return &SpanSet{m: map[[2]uint32]struct{}{}}
}

func (s *SpanSet) ensure() {
	if s.m == nil {
		s.m = map[[2]uint32]struct{}{}
	}
}

// Add records sp as occupied. Empty spans are ignored.
func (s *SpanSet) Add(sp ingestutil.Span) {
	if s == nil || sp.Empty() {
		return
	}
	s.ensure()
	s.m[[2]uint32{sp.StartByte, sp.EndByte}] = struct{}{}
}

// AddRange records [start,end) as occupied.
func (s *SpanSet) AddRange(start, end uint32) {
	s.Add(ingestutil.Span{StartByte: start, EndByte: end})
}

// Overlaps reports whether sp intersects any span in the set (or equals one).
// A nil receiver never overlaps.
func (s *SpanSet) Overlaps(sp ingestutil.Span) bool {
	return s.OverlapsRange(sp.StartByte, sp.EndByte)
}

// OverlapsRange reports whether [start,end) intersects any occupied span.
func (s *SpanSet) OverlapsRange(start, end uint32) bool {
	if s == nil || s.m == nil || start >= end {
		return false
	}
	if _, ok := s.m[[2]uint32{start, end}]; ok {
		return true
	}
	for k := range s.m {
		if start < k[1] && end > k[0] {
			return true
		}
	}
	return false
}

// TryAdd records sp if it does not overlap; returns true when recorded.
func (s *SpanSet) TryAdd(sp ingestutil.Span) bool {
	if s == nil || sp.Empty() || s.Overlaps(sp) {
		return false
	}
	s.Add(sp)
	return true
}

// FileSpanSet maps project-relative file paths (no leading "./") to SpanSets.
// A nil map is empty. Prefer For/Mark/Overlaps helpers so path form is consistent.
type FileSpanSet map[string]*SpanSet

// cleanFilePath normalizes to project-relative without "./".
func cleanFilePath(file string) string {
	return strings.TrimPrefix(file, "./")
}

// For returns the SpanSet for file, creating it if needed.
// Panics if f is nil.
func (f FileSpanSet) For(file string) *SpanSet {
	file = cleanFilePath(file)
	if f[file] == nil {
		f[file] = NewSpanSet()
	}
	return f[file]
}

// Overlaps reports whether sp is occupied in file.
func (f FileSpanSet) Overlaps(file string, sp ingestutil.Span) bool {
	if f == nil {
		return false
	}
	return f[cleanFilePath(file)].Overlaps(sp)
}

// OverlapsRange reports whether [start,end) is occupied in file.
func (f FileSpanSet) OverlapsRange(file string, start, end uint32) bool {
	return f.Overlaps(file, ingestutil.Span{StartByte: start, EndByte: end})
}

// Mark records sp as occupied in file (creates the file set if needed).
func (f FileSpanSet) Mark(file string, sp ingestutil.Span) {
	if f == nil {
		return
	}
	f.For(file).Add(sp)
}

// MarkRange records [start,end) as occupied in file.
func (f FileSpanSet) MarkRange(file string, start, end uint32) {
	f.Mark(file, ingestutil.Span{StartByte: start, EndByte: end})
}

// TryMark records sp if free; returns true when newly marked.
func (f FileSpanSet) TryMark(file string, sp ingestutil.Span) bool {
	if f == nil {
		return false
	}
	return f.For(file).TryAdd(sp)
}
