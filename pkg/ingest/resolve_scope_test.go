package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"
)

func TestScopeTree_ReplacesAncestorWalk(t *testing.T) {
	fe := &project.FileExtract{
		Path: "Types.java",
		Scopes: []project.ScopeDef{
			{StartByte: 10, EndByte: 100, Parent: -1},
			{StartByte: 100, EndByte: 200, Parent: -1},
			{StartByte: 120, EndByte: 180, Parent: 1},
		},
		Atoms: []project.AtomDef{
			{Name: "A.value", StartByte: 10, EndByte: 15, ScopeIdx: 0},
			{Name: "B.value", StartByte: 110, EndByte: 115, ScopeIdx: 1},
		},
	}
	tr := buildScopeTree(fe, &refCache{})
	{
		got := tr.Lookup(130, "value")
		require.Equal(t, "path:./Types.java::B.value", got,
			"B.m value=%q", got)
	}
	{

		got := tr.Lookup(10, "value")
		require.Equal(t, "path:./Types.java::A.value", got,
			"A value=%q", got)
	}

	got := tr.Lookup(5, "value")
	require.Empty(t, got,
		"file value=%q want empty", got)

}

func TestScopeTree_LocalShadowsField(t *testing.T) {
	fe := &project.FileExtract{
		Path: "T.java",
		Scopes: []project.ScopeDef{
			{StartByte: 0, EndByte: 200, Parent: -1},
			{StartByte: 80, EndByte: 180, Parent: 0},
		},
		Atoms: []project.AtomDef{
			{Name: "T.map", StartByte: 20, EndByte: 23, ScopeIdx: 0},
			{Name: "T.get", StartByte: 82, EndByte: 85, ScopeIdx: 1},
			{Name: "map", StartByte: 90, EndByte: 93, ScopeIdx: 1},
		},
	}
	tr := buildScopeTree(fe, &refCache{})
	got := tr.Lookup(90, "map")
	require.Equal(t, "path:./T.java::map", got,
		"local map=%q", got)

}
