package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"
)

func TestInsertAt(t *testing.T) {
	e := InsertAt("./f.go", 3, "x")
	require.Equal(t, "f.go", e.File)
	require.Equal(t, uint32(3), e.StartByte)
	require.Equal(t, uint32(3), e.EndByte)
	require.Equal(t, "x", e.NewText)
	require.Equal(t, project.Edit{}, InsertAt("f", 0, ""))

}

func TestInsertLinesAt(t *testing.T) {
	edits := InsertLinesAt("f.go", 0, []string{"import A", "import B"})
	require.Len(t, edits, 1)
	require.Equal(t, "import A\nimport B\n", edits[0].NewText)
	require.Nil(t, InsertLinesAt("f.go", 0, nil))

}

func TestLocalDepsInDeclSpan(t *testing.T) {
	result := &project.Result{
		Atoms: []project.Atom{
			{Reference: "path:./a.js::moved"},
			{Reference: "path:./a.js::helper"},
			{Reference: "path:./a.js::Cls.m"},
			{Reference: "path:./b.js::other"},
		},
		Uses: []project.Use{
			{Reference: "path:./a.js", StartByte: 10, EndByte: 16, Target: "path:./a.js::helper"},
			{Reference: "path:./a.js", StartByte: 20, EndByte: 25, Target: "path:./a.js::Cls.m"},
			{Reference: "path:./a.js", StartByte: 100, EndByte: 110, Target: "path:./a.js::helper"}, // outside span
		},
	}
	src := ParseReference("path:./a.js::moved")
	decl := DeclExtract{RemoveStart: 0, RemoveEnd: 50}
	deps := LocalDepsInDeclSpan(result, src, decl, LocalDepOpts{})
	require.Len(t, deps, 2)

	deps = LocalDepsInDeclSpan(result, src, decl, LocalDepOpts{TopLevelOnly: true})
	require.Equal(t, []string{"helper"}, deps)

}
