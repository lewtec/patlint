package ingest_test

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
)

func TestReplaceSpan_SkipsEmpty(t *testing.T) {
	e := ingest.ReplaceSpan("a.go", ingestutil.Span{StartByte: 5, EndByte: 5}, "x")
	require.Empty(t, e.File, "expected zero edit for empty span, got %+v", e)
}

func TestReplaceSpans(t *testing.T) {
	spans := []ingestutil.Span{
		{StartByte: 0, EndByte: 3},
		{StartByte: 10, EndByte: 10}, // skipped
		{StartByte: 4, EndByte: 7},
	}
	edits := ingest.ReplaceSpans("./pkg/a.go", spans, "New")
	require.Len(t, edits, 2)
	require.Equal(t, "pkg/a.go", edits[0].File)
	require.Equal(t, "New", edits[0].NewText)
	require.Equal(t, uint32(0), edits[0].StartByte)
	require.Equal(t, uint32(3), edits[0].EndByte)
	require.Equal(t, uint32(4), edits[1].StartByte)
	require.Equal(t, uint32(7), edits[1].EndByte)
}

func TestAppendReplaceSpan(t *testing.T) {
	var edits []project.Edit
	edits = ingest.AppendReplaceSpan(edits, "f.go", ingestutil.Span{StartByte: 1, EndByte: 2}, "a")
	edits = ingest.AppendReplaceSpan(edits, "f.go", ingestutil.Span{}, "b")
	require.Len(t, edits, 1)
	require.Equal(t, "a", edits[0].NewText)
}
