package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
)

func TestOutlineFromExtract_NestsByScope(t *testing.T) {
	fe := &project.FileExtract{
		Scopes: []project.ScopeDef{
			{StartByte: 10, EndByte: 80, Parent: -1},  // type T
			{StartByte: 90, EndByte: 140, Parent: -1}, // func F
		},
		Atoms: []project.AtomDef{
			{Name: "T", StartByte: 15, EndByte: 16, ScopeIdx: 0},
			{Name: "T.F", StartByte: 30, EndByte: 31, ScopeIdx: 0},
			{Name: "F", StartByte: 95, EndByte: 96, ScopeIdx: 1},
			{Name: "Pkg", StartByte: 0, EndByte: 3, ScopeIdx: -1},
		},
		Imports: []project.ImportDef{
			{LocalName: "fmt", StartByte: 5, EndByte: 8},
		},
	}
	got := OutlineFromExtract(fe)
	want := []OutlineRow{
		{Depth: 0, Name: "Pkg", Kind: "def", StartByte: 0, EndByte: 3},
		{Depth: 0, Name: "fmt", Kind: "import", StartByte: 5, EndByte: 8},
		{Depth: 0, Name: "T", Kind: "def", StartByte: 15, EndByte: 16},
		{Depth: 1, Name: "T.F", Kind: "def", StartByte: 30, EndByte: 31},
		{Depth: 0, Name: "F", Kind: "def", StartByte: 95, EndByte: 96},
	}
	if len(got) != len(want) {
		t.Fatalf("rows=%d want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Depth != want[i].Depth || got[i].Name != want[i].Name || got[i].Kind != want[i].Kind {
			t.Fatalf("row %d = %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestOutlineFromExtract_NestedScopes(t *testing.T) {
	fe := &project.FileExtract{
		Scopes: []project.ScopeDef{
			{StartByte: 0, EndByte: 100, Parent: -1},
			{StartByte: 20, EndByte: 80, Parent: 0},
		},
		Atoms: []project.AtomDef{
			{Name: "C", StartByte: 5, EndByte: 6, ScopeIdx: 0},
			{Name: "m", StartByte: 25, EndByte: 26, ScopeIdx: 1},
		},
	}
	got := OutlineFromExtract(fe)
	if len(got) != 2 || got[0].Name != "C" || got[0].Depth != 0 || got[1].Name != "m" || got[1].Depth != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestOutlineFromExtract_EmptyScopesDropped(t *testing.T) {
	fe := &project.FileExtract{
		Scopes: []project.ScopeDef{
			{StartByte: 0, EndByte: 10, Parent: -1},
		},
		Atoms: []project.AtomDef{
			{Name: "X", StartByte: 20, EndByte: 21, ScopeIdx: -1},
		},
	}
	got := OutlineFromExtract(fe)
	if len(got) != 1 || got[0].Name != "X" || got[0].Depth != 0 {
		t.Fatalf("got %+v", got)
	}
}
