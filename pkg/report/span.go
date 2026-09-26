package report

import (
	"fmt"
	"github.com/lewtec/lewkit/x/text"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"strings"
)

// SpanLoc maps a half-open byte span to 1-based line/column and a one-line snippet.
func SpanLoc(src []byte, sp ingestutil.Span) (line, col, endLine, endCol int, snippet string, err error) {
	if int(sp.EndByte) > len(src) || sp.StartByte > sp.EndByte {
		return 0, 0, 0, 0, "", fmt.Errorf("span out of range")
	}
	li := text.NewLineIndexBytes(src)
	l, c0 := li.LineColumnAtU32(sp.StartByte)
	el, ec0 := li.LineColumnAtU32(sp.EndByte)
	text := string(src[sp.StartByte:sp.EndByte])
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i] + "…"
	}
	return l, c0 + 1, el, ec0 + 1, text, nil
}

// OneLine collapses s to a single display line (trim; first line + "…" if multi-line).
func OneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i]) + "…"
	}
	return s
}

// EditsOverlapAny reports whether any edit's span overlaps claimed.
func EditsOverlapAny(edits []project.Edit, claimed []ingestutil.Span) bool {
	for _, e := range edits {
		if ingestutil.OverlapsAny(e.Span, claimed) {
			return true
		}
	}
	return false
}

// EditBodySpans returns non-empty old ranges from edits (insert-only skipped).
func EditBodySpans(edits []project.Edit) []ingestutil.Span {
	if len(edits) == 0 {
		return nil
	}
	out := make([]ingestutil.Span, 0, len(edits))
	for _, e := range edits {
		if !e.Span.Empty() {
			out = append(out, e.Span)
		}
	}
	return out
}
