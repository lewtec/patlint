package store

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
)

func TestInsertDedupAndAppend(t *testing.T) {
	s := New()
	if !s.Insert(RelationFile, Tuple{"a.go", "go", "main"}) {
		t.Fatal("first insert")
	}
	if s.Insert(RelationFile, Tuple{"a.go", "go", "main"}) {
		t.Fatal("duplicate insert")
	}
	s.Append(RelationBinds, Tuple{"a.go", "1", "2", "from", "to", "0"})
	s.Append(RelationBinds, Tuple{"a.go", "1", "2", "from", "to", "0"})
	if n := len(s.Rows(RelationBinds)); n != 2 {
		t.Fatalf("append binds got %d", n)
	}
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
	if got == nil || got.Package != "main" || got.Language != "go" {
		t.Fatalf("file: %+v", got)
	}
	if len(got.Atoms) != 1 || got.Atoms[0].Name != "helper" {
		t.Fatalf("atoms: %+v", got.Atoms)
	}
	if len(got.Imports) != 1 || got.Imports[0].SourcePath != "fmt" {
		t.Fatalf("imports: %+v", got.Imports)
	}
	if len(got.Usages) != 1 || got.Usages[0].Name != "Println" || len(got.Usages[0].Prefix) != 1 {
		t.Fatalf("uses: %+v", got.Usages)
	}
	if len(got.Scopes) != 1 || got.Scopes[0].EndByte != 80 {
		t.Fatalf("scopes: %+v", got.Scopes)
	}
	if len(got.Flows) != 1 || got.Flows[0].Class != project.FlowStructural || got.Flows[0].StartByte != 30 {
		t.Fatalf("flows: %+v", got.Flows)
	}
}
