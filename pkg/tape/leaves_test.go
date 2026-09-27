package tape_test

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
	"github.com/lewtec/patlint/pkg/tape"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, `"hi"`, saw, "cells=%v", texts(src, cells))
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
	require.False(t, fSpan.Empty(), "no F leaf")

	targets := map[tape.Span]string{fSpan: "path:./x.go::F"}
	cells := tape.Build(root, src, tape.DefaultPolicy(), targets)
	found := false
	for _, c := range cells {
		if c.Span == fSpan {
			require.Equal(t, "path:./x.go::F", c.Target)
			found = true
		}
	}
	require.True(t, found, "F cell missing after Build")
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
	require.Fail(t, "expected interface{} as one cell", "got %v", texts(src, cells))
}

func parseGo(t *testing.T, src []byte) *sitter.Node {
	t.Helper()
	pf, err := ingestutil.ParseSource(t.Context(), treesitter.Engine{}, src, "x.go", "go")
	require.NoError(t, err)
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
	require.NotContains(t, types, "outer")
	require.Contains(t, types, "inner")
	require.Contains(t, types, "other")
}
