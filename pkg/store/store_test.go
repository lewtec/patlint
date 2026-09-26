package store

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"
)

func TestInsertDedupAndAppend(t *testing.T) {
	s := New()
	require.True(t, s.Insert(RelationFile, Tuple{"a.go", "go", "main"}),
		"first insert")
	require.False(t, s.Insert(RelationFile, Tuple{"a.go", "go", "main"}),
		"duplicate insert")

	s.Append(RelationBinds, Tuple{"a.go", "1", "2", "from", "to", "0"})
	s.Append(RelationBinds, Tuple{"a.go", "1", "2", "from", "to", "0"})
	require.Len(t, s.Rows(RelationBinds), 2)
}

func TestProjectExtractRoundTrip(t *testing.T) {
	fe := &project.FileExtract{
		Language:   "go",
		Path:       "main.go",
		Package:    "main",
		PackageEnd: 12,
		Atoms: []project.AtomDef{{
			Name: "helper", StartByte: 20, EndByte: 26, ScopeIdx: -1,
		}},
		Imports: []project.ImportDef{{
			LocalName: "fmt", SourcePath: "fmt", StartByte: 13, EndByte: 24,
			PathStartByte: 20, PathEndByte: 23,
		}},
		Usages: []project.UsageDef{{
			Name: "Println", StartByte: 40, EndByte: 47, ScopeIdx: -1,
			Prefix: []project.UsageName{{Name: "fmt", StartByte: 36, EndByte: 39}},
		}},
		Scopes: []project.ScopeDef{{StartByte: 14, EndByte: 80, Parent: -1}},
		Flows: []project.FlowDef{{
			StartByte: 30, EndByte: 50, Class: project.FlowStructural,
		}},
	}
	s := New()
	LoadIngest(s, []*project.FileExtract{fe}, nil, nil)
	got := ProjectExtract(s, "main.go")
	require.NotNil(t, got)
	require.Equal(t, "main", got.Package)
	require.Equal(t, "go", got.Language)
	require.Len(t, got.Atoms, 1)
	require.Equal(t, "helper", got.Atoms[0].Name)
	require.Len(t, got.Imports, 1)
	require.Equal(t, "fmt", got.Imports[0].SourcePath)
	require.Len(t, got.Usages, 1)
	require.Equal(t, "Println", got.Usages[0].Name)
	require.Len(t, got.Usages[0].Prefix, 1)
	require.Len(t, got.Scopes, 1)
	require.Equal(t, uint32(80), got.Scopes[0].EndByte)
	require.Len(t, got.Flows, 1)
	require.Equal(t, project.FlowStructural, got.Flows[0].Class)
	require.Equal(t, uint32(30), got.Flows[0].StartByte)
}
