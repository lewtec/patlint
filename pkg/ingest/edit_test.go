package ingest_test

import (
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"testing"

	"github.com/lewtec/patlint/pkg/ingest"
)

func TestReplaceSpan_SkipsEmpty(t *testing.T) {
	e := ingest.ReplaceSpan("a.go", ingestutil.Span{StartByte: 5, EndByte: 5}, "x")
	if e.File != "" {
		t.Fatalf("expected zero edit for empty span, got %+v", e)
	}
}

func TestReplaceSpans(t *testing.T) {
	spans := []ingestutil.Span{
		{StartByte: 0, EndByte: 3},
		{StartByte: 10, EndByte: 10}, // skipped
		{StartByte: 4, EndByte: 7},
	}
	edits := ingest.ReplaceSpans("./pkg/a.go", spans, "New")
	if len(edits) != 2 {
		t.Fatalf("len=%d want 2: %+v", len(edits), edits)
	}
	if edits[0].File != "pkg/a.go" || edits[0].NewText != "New" {
		t.Fatalf("edit0=%+v", edits[0])
	}
	if edits[0].StartByte != 0 || edits[0].EndByte != 3 {
		t.Fatalf("edit0 span=%v", edits[0].Span)
	}
	if edits[1].StartByte != 4 || edits[1].EndByte != 7 {
		t.Fatalf("edit1 span=%v", edits[1].Span)
	}
}

func TestAppendReplaceSpan(t *testing.T) {
	var edits []project.Edit
	edits = ingest.AppendReplaceSpan(edits, "f.go", ingestutil.Span{StartByte: 1, EndByte: 2}, "a")
	edits = ingest.AppendReplaceSpan(edits, "f.go", ingestutil.Span{}, "b")
	if len(edits) != 1 || edits[0].NewText != "a" {
		t.Fatalf("%+v", edits)
	}
}
