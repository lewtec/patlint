package tape

// Span is a half-open UTF-8 byte range [StartByte, EndByte) in a source buffer.
// Same contract as ingestutil.Span; duplicated here so tape does not import ingest.
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

// Bytes returns src[StartByte:EndByte], or nil if out of range / invalid.
func (s Span) Bytes(src []byte) []byte {
	if src == nil || s.EndByte > uint32(len(src)) || s.StartByte > s.EndByte {
		return nil
	}
	return src[s.StartByte:s.EndByte]
}

// Empty reports whether the span has zero length.
func (s Span) Empty() bool { return s.StartByte >= s.EndByte }

// Cell is one significant leaf on the structural tape.
// Text is never stored; use Span.Text(source) / Span.Bytes(source).
type Cell struct {
	Span
	// Type is the tree-sitter node type for this leaf (empty if synthetic).
	Type string
	// Target is an optional resolved product reference (e.g. use hyperlink).
	Target string
	// TokenClass is an optional product highlight class (tok-kw, tok-str, …).
	// Filled by pack-driven tape build (pattern.BuildTape); empty if unset.
	TokenClass string
}
