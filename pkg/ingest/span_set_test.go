package ingest

import (
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

import "testing"

func TestSpanSetOverlaps(t *testing.T) {
	var nilSet *SpanSet
	if nilSet.Overlaps(ingestutil.Span{0, 1}) {
		t.Fatal("nil set")
	}
	s := NewSpanSet()
	s.Add(ingestutil.Span{10, 20})
	if !s.Overlaps(ingestutil.Span{10, 20}) {
		t.Fatal("exact")
	}
	if !s.Overlaps(ingestutil.Span{15, 18}) {
		t.Fatal("interior")
	}
	if !s.Overlaps(ingestutil.Span{5, 12}) {
		t.Fatal("partial left")
	}
	if s.Overlaps(ingestutil.Span{20, 25}) {
		t.Fatal("abutting end exclusive")
	}
	if s.Overlaps(ingestutil.Span{0, 10}) {
		t.Fatal("abutting start exclusive")
	}
	if s.TryAdd(ingestutil.Span{12, 14}) {
		t.Fatal("overlap try")
	}
	if !s.TryAdd(ingestutil.Span{30, 40}) {
		t.Fatal("free try")
	}
	s.Add(ingestutil.Span{}) // empty ignored
	if s.Overlaps(ingestutil.Span{0, 0}) {
		t.Fatal("empty query")
	}
}

func TestFileSpanSet(t *testing.T) {
	f := FileSpanSet{}
	f.Mark("./pkg/a.go", ingestutil.Span{1, 2})
	if !f.Overlaps("pkg/a.go", ingestutil.Span{1, 2}) {
		t.Fatal("path normalize")
	}
	if f.Overlaps("pkg/b.go", ingestutil.Span{1, 2}) {
		t.Fatal("other file")
	}
	if !f.TryMark("pkg/b.go", ingestutil.Span{5, 6}) {
		t.Fatal("new file")
	}
	if f.TryMark("pkg/b.go", ingestutil.Span{5, 6}) {
		t.Fatal("dup")
	}
}

func TestMarkEntityRelationSpansFileSpanSet(t *testing.T) {
	result := &project.Result{
		Atoms: []project.Atom{{Reference: "path:./a.go::Foo", StartByte: 1, EndByte: 4}},
		Uses:  []project.Use{{Reference: "path:./b.go", Target: "path:./a.go::Foo", StartByte: 10, EndByte: 13}},
	}
	occ := MarkEntityRelationSpans(result, NewStringSet("path:./a.go::Foo"))
	if !occ.Overlaps("a.go", ingestutil.Span{1, 4}) {
		t.Fatal("atom")
	}
	if !occ.Overlaps("b.go", ingestutil.Span{10, 13}) {
		t.Fatal("use")
	}
}

func TestAppendUnoccupiedSpanSet(t *testing.T) {
	occ := NewSpanSet()
	e1 := project.Edit{Span: ingestutil.Span{0, 2}, NewText: "x"}
	edits := AppendUnoccupied(nil, occ, e1)
	if len(edits) != 1 {
		t.Fatal(edits)
	}
	edits = AppendUnoccupied(edits, occ, project.Edit{Span: ingestutil.Span{0, 1}, NewText: "y"})
	if len(edits) != 1 {
		t.Fatal("overlap")
	}
	edits = AppendUnoccupiedAll(edits, occ, []project.Edit{{Span: ingestutil.Span{5, 6}, NewText: "z"}})
	if len(edits) != 2 {
		t.Fatal(edits)
	}
}
