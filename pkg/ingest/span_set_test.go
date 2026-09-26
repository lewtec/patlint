package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"
)

func TestSpanSetOverlaps(t *testing.T) {
	var nilSet *SpanSet
	require.False(t, nilSet.Overlaps(ingestutil.Span{0, 1}),
		"nil set")

	s := NewSpanSet()
	s.Add(ingestutil.Span{10, 20})
	require.True(t, s.Overlaps(ingestutil.Span{10, 20}),
		"exact")
	require.True(t, s.Overlaps(ingestutil.Span{15, 18}),
		"interior")
	require.True(t, s.Overlaps(ingestutil.Span{5, 12}),
		"partial left")
	require.False(t, s.Overlaps(ingestutil.Span{20, 25}),
		"abutting end exclusive")
	require.False(t, s.Overlaps(ingestutil.Span{0, 10}),
		"abutting start exclusive")
	require.False(t, s.TryAdd(ingestutil.Span{12, 14}),
		"overlap try")
	require.True(t, s.TryAdd(ingestutil.Span{30, 40}),
		"free try")

	s.Add(ingestutil.Span{})
	require. // empty ignored
			False(t, s.Overlaps(ingestutil.Span{0, 0}),
			"empty query")

}

func TestFileSpanSet(t *testing.T) {
	f := FileSpanSet{}
	f.Mark("./pkg/a.go", ingestutil.Span{1, 2})
	require.True(t, f.Overlaps("pkg/a.go", ingestutil.Span{1, 2}),
		"path normalize")
	require.False(t, f.Overlaps("pkg/b.go", ingestutil.Span{1, 2}),
		"other file")
	require.True(t, f.TryMark("pkg/b.go", ingestutil.Span{5, 6}),
		"new file")
	require.False(t, f.TryMark("pkg/b.go", ingestutil.Span{5, 6}),
		"dup")

}

func TestMarkEntityRelationSpansFileSpanSet(t *testing.T) {
	result := &project.Result{
		Atoms: []project.Atom{{Reference: "path:./a.go::Foo", StartByte: 1, EndByte: 4}},
		Uses:  []project.Use{{Reference: "path:./b.go", Target: "path:./a.go::Foo", StartByte: 10, EndByte: 13}},
	}
	occ := MarkEntityRelationSpans(result, NewStringSet("path:./a.go::Foo"))
	require.True(t, occ.Overlaps("a.go", ingestutil.Span{1, 4}),
		"atom")
	require.True(t, occ.Overlaps("b.go", ingestutil.Span{10, 13}),
		"use")

}

func TestAppendUnoccupiedSpanSet(t *testing.T) {
	occ := NewSpanSet()
	e1 := project.Edit{Span: ingestutil.Span{0, 2}, NewText: "x"}
	edits := AppendUnoccupied(nil, occ, e1)
	require.Len(t, edits, 1)

	edits = AppendUnoccupied(edits, occ, project.Edit{Span: ingestutil.Span{0, 1}, NewText: "y"})
	require.Len(t, edits, 1,
		"overlap")

	edits = AppendUnoccupiedAll(edits, occ, []project.Edit{{Span: ingestutil.Span{5, 6}, NewText: "z"}})
	require.Len(t, edits, 2)

}
