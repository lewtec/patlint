package ingestutil

// Span is a half-open UTF-8 byte range [StartByte, EndByte) in a source buffer.
// Offsets match tree-sitter StartByte/EndByte (not rune indexes).
type Span struct {
	StartByte uint32
	EndByte   uint32
}

// Text returns src[StartByte:EndByte].
func (s Span) Text(src []byte) string {
	b := s.Bytes(src)
	if b == nil {
		return ""
	}
	return string(b)
}

// Bytes returns src[StartByte:EndByte], or nil if out of range / empty invalid.
func (s Span) Bytes(src []byte) []byte {
	if src == nil || s.EndByte > uint32(len(src)) || s.StartByte > s.EndByte {
		return nil
	}
	return src[s.StartByte:s.EndByte]
}

// Empty reports whether the span has zero length (StartByte >= EndByte).
func (s Span) Empty() bool { return s.StartByte >= s.EndByte }

// Overlaps reports half-open range overlap [a,b) ∩ [c,d) ≠ ∅.
func (a Span) Overlaps(b Span) bool {
	return a.StartByte < b.EndByte && b.StartByte < a.EndByte
}

// Eq reports whether the span's bytes equal want without allocating a string.
func (s Span) Eq(src []byte, want string) bool {
	b := s.Bytes(src)
	if len(b) != len(want) {
		return false
	}
	for i := 0; i < len(b); i++ {
		if b[i] != want[i] {
			return false
		}
	}
	return true
}

// OverlapsAny reports whether sp overlaps any of claimed.
func OverlapsAny(sp Span, claimed []Span) bool {
	for _, c := range claimed {
		if sp.Overlaps(c) {
			return true
		}
	}
	return false
}

// AnyOverlapsAny reports whether any span in a overlaps any in b.
func AnyOverlapsAny(a, b []Span) bool {
	for _, sp := range a {
		if OverlapsAny(sp, b) {
			return true
		}
	}
	return false
}

// NonEmpty returns the non-empty spans from ss (order preserved).
func NonEmpty(ss []Span) []Span {
	if len(ss) == 0 {
		return nil
	}
	out := make([]Span, 0, len(ss))
	for _, s := range ss {
		if !s.Empty() {
			out = append(out, s)
		}
	}
	return out
}
