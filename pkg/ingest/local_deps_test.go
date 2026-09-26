package ingest

import (
	"github.com/lewtec/patlint/pkg/project"
	"testing"
)

func TestInsertAt(t *testing.T) {
	e := InsertAt("./f.go", 3, "x")
	if e.File != "f.go" || e.StartByte != 3 || e.EndByte != 3 || e.NewText != "x" {
		t.Fatalf("%+v", e)
	}
	if InsertAt("f", 0, "") != (project.Edit{}) {
		t.Fatal("empty text")
	}
}

func TestInsertLinesAt(t *testing.T) {
	edits := InsertLinesAt("f.go", 0, []string{"import A", "import B"})
	if len(edits) != 1 || edits[0].NewText != "import A\nimport B\n" {
		t.Fatalf("%+v", edits)
	}
	if InsertLinesAt("f.go", 0, nil) != nil {
		t.Fatal("nil lines")
	}
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
	if len(deps) != 2 {
		t.Fatalf("all names: %v", deps)
	}
	deps = LocalDepsInDeclSpan(result, src, decl, LocalDepOpts{TopLevelOnly: true})
	if len(deps) != 1 || deps[0] != "helper" {
		t.Fatalf("top-level: %v", deps)
	}
}
