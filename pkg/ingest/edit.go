package ingest

import (
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	// ReplaceSpan builds one leaf/text replacement edit at sp in file.
	// file is stored without a leading "./". Empty or inverted spans are skipped
	// (returns a zero Edit with empty File).
)

import "strings"

func ReplaceSpan(file string, sp ingestutil.Span, newText string) project.Edit {
	if sp.Empty() || sp.StartByte > sp.EndByte {
		return project.Edit{}
	}
	return project.Edit{
		File:    strings.TrimPrefix(file, "./"),
		Span:    sp,
		NewText: newText,
	}
}

// ReplaceSpans builds edits that write newText over each span in file.
// Skips empty/inverted spans. Order follows spans.
func ReplaceSpans(file string, spans []ingestutil.Span, newText string) []project.Edit {
	if len(spans) == 0 {
		return nil
	}
	file = strings.TrimPrefix(file, "./")
	edits := make([]project.Edit, 0, len(spans))
	for _, sp := range spans {
		if sp.Empty() || sp.StartByte > sp.EndByte {
			continue
		}
		edits = append(edits, project.Edit{File: file, Span: sp, NewText: newText})
	}
	return edits
}

// AppendReplaceSpan appends a ReplaceSpan edit when the span is non-empty.
func AppendReplaceSpan(edits []project.Edit, file string, sp ingestutil.Span, newText string) []project.Edit {
	e := ReplaceSpan(file, sp, newText)
	if e.File == "" {
		return edits
	}
	return append(edits, e)
}
