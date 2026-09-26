package tape_test

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/tape"
)

func TestLeaves_GoStringAtomic(t *testing.T) {
	src := []byte(`package p; var s = "hi"`)
	root := parseGo(t, src)
	cells := tape.Leaves(root, src, tape.DefaultPolicy())
	var saw string
	for _, c := range cells {
		if c.Type == "interpreted_string_literal" {
			saw = c.Text(src)
			break
		}
	}
	if saw != `"hi"` {
		t.Fatalf("string cell=%q cells=%v", saw, texts(src, cells))
	}
}

func TestBuild_AttachTarget(t *testing.T) {
	src := []byte("package p\nfunc F() {}\n")
	root := parseGo(t, src)
	var fSpan tape.Span
	for _, c := range tape.Leaves(root, src, tape.DefaultPolicy()) {
		if c.Text(src) == "F" {
			fSpan = c.Span
			break
		}
	}
	if fSpan.Empty() {
		t.Fatal("no F leaf")
	}
	targets := map[tape.Span]string{fSpan: "path:./x.go::F"}
	cells := tape.Build(root, src, tape.DefaultPolicy(), targets)
	found := false
	for _, c := range cells {
		if c.Span == fSpan {
			if c.Target != "path:./x.go::F" {
				t.Fatalf("target=%q", c.Target)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("F cell missing after Build")
	}
}

func TestBuild_GoCompositeWithPolicy(t *testing.T) {
	src := []byte("package p\nfunc f(x interface{}) {}\n")
	root := parseGo(t, src)
	cells := tape.Build(root, src, tape.WithAtomicTexts([]string{"interface{}"}), nil)
	for _, c := range cells {
		if c.Text(src) == "interface{}" {
			return
		}
	}
	t.Fatalf("expected interface{} as one cell; got %v", texts(src, cells))
}

func parseGo(t *testing.T, src []byte) *sitter.Node {
	t.Helper()
	pf, err := ingestutil.ParseSource(t.Context(), ccgo.Engine{}, src, "x.go", "go")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pf.Close() })
	return pf.Root
}

func texts(src []byte, cells []tape.Cell) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = c.Text(src)
	}
	return out
}

func TestPreferInnermost_DropsOuter(t *testing.T) {
	// Outer [0,10) contains inner [2,5); only inner (and disjoint) should remain.
	cands := []tape.Cell{
		{Span: tape.Span{StartByte: 0, EndByte: 10}, Type: "outer"},
		{Span: tape.Span{StartByte: 2, EndByte: 5}, Type: "inner"},
		{Span: tape.Span{StartByte: 20, EndByte: 22}, Type: "other"},
	}
	// Build-style finalize path: use exported Build with empty root impossible.
	// Test via package-level by building from Leaves only — call Finalize.
	out := tape.Finalize(cands, []byte("xxxxxxxxxx..........xx"), nil)
	var types []string
	for _, c := range out {
		types = append(types, c.Type)
	}
	// outer dropped
	for _, typ := range types {
		if typ == "outer" {
			t.Fatalf("outer should be dropped: %v", types)
		}
	}
	sawInner, sawOther := false, false
	for _, typ := range types {
		if typ == "inner" {
			sawInner = true
		}
		if typ == "other" {
			sawOther = true
		}
	}
	if !sawInner || !sawOther {
		t.Fatalf("want inner+other, got %v", types)
	}
}
