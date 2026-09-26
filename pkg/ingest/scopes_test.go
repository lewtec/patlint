package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
)

func TestScopeTree_AtNested(t *testing.T) {
	fe := &project.FileExtract{
		Path: "t.go",
		Scopes: []project.ScopeDef{
			{StartByte: 0, EndByte: 100, Parent: -1}, // class
			{StartByte: 20, EndByte: 50, Parent: 0},  // method
		},
	}
	tr := buildScopeTree(fe, &refCache{})
	if got := tr.At(0); got != 0 {
		t.Fatalf("At(0)=%d want 0", got)
	}
	if got := tr.At(25); got != 1 {
		t.Fatalf("At(25)=%d want 1", got)
	}
	if got := tr.At(80); got != 0 {
		t.Fatalf("At(80)=%d want 0", got)
	}
}

func TestScopeTree_LookupShadows(t *testing.T) {
	fe := &project.FileExtract{
		Path: "Types.java",
		Scopes: []project.ScopeDef{
			{StartByte: 10, EndByte: 100, Parent: -1},  // A
			{StartByte: 100, EndByte: 200, Parent: -1}, // B
			{StartByte: 120, EndByte: 180, Parent: 1},  // B.m
		},
		Atoms: []project.AtomDef{
			{Name: "A", StartByte: 1, EndByte: 2, ScopeIdx: 0},
			{Name: "A.run", StartByte: 10, EndByte: 13, ScopeIdx: 0},
			{Name: "B", StartByte: 101, EndByte: 102, ScopeIdx: 1},
			{Name: "B.run", StartByte: 110, EndByte: 113, ScopeIdx: 1},
			{Name: "x", StartByte: 130, EndByte: 131, ScopeIdx: 2},
		},
	}
	tr := buildScopeTree(fe, &refCache{})
	// In B.m, local x is visible; run is B.run (class), not A.run.
	if got := tr.Lookup(130, "x"); got != "path:./Types.java::x" {
		t.Fatalf("x=%q", got)
	}
	if got := tr.Lookup(130, "run"); got != "path:./Types.java::B.run" {
		t.Fatalf("run in B.m=%q", got)
	}
	if got := tr.Lookup(10, "run"); got != "path:./Types.java::A.run" {
		t.Fatalf("run in A=%q", got)
	}
	// File-level: types, not methods.
	if got := tr.Lookup(5, "A"); got != "path:./Types.java::A" {
		t.Fatalf("A=%q", got)
	}
	if got := tr.Lookup(5, "run"); got != "" {
		t.Fatalf("file run=%q want empty", got)
	}
}

func TestScopeTree_TypeInNamespaceVisibleToSibling(t *testing.T) {
	fe := &project.FileExtract{
		Path: "features.cpp",
		Scopes: []project.ScopeDef{
			{StartByte: 0, EndByte: 80, Parent: -1}, // namespace
			{StartByte: 10, EndByte: 30, Parent: 0}, // Base
			{StartByte: 31, EndByte: 70, Parent: 0}, // Box
		},
		Atoms: []project.AtomDef{
			{Name: "util", StartByte: 1, EndByte: 5, ScopeIdx: 0},
			{Name: "Base", StartByte: 16, EndByte: 20, ScopeIdx: 1},
			{Name: "Box", StartByte: 37, EndByte: 40, ScopeIdx: 2},
		},
	}
	tr := buildScopeTree(fe, &refCache{})
	if got := tr.Lookup(45, "Base"); got != "path:./features.cpp::Base" {
		t.Fatalf("Base from Box=%q", got)
	}
}

func TestScopeTree_DottedLocalStaysInMethod(t *testing.T) {
	fe := &project.FileExtract{
		Path: "Main.java",
		Scopes: []project.ScopeDef{
			{StartByte: 0, EndByte: 300, Parent: -1},  // Main
			{StartByte: 40, EndByte: 120, Parent: 0},  // use
			{StartByte: 130, EndByte: 280, Parent: 0}, // main
		},
		Atoms: []project.AtomDef{
			{Name: "Main", StartByte: 10, EndByte: 14, ScopeIdx: 0},
			{Name: "Main.use", StartByte: 50, EndByte: 53, ScopeIdx: 1},
			{Name: "Main.use.m", StartByte: 60, EndByte: 61, ScopeIdx: 1},
			{Name: "Main.main", StartByte: 140, EndByte: 144, ScopeIdx: 2},
			{Name: "Main.main.m", StartByte: 200, EndByte: 201, ScopeIdx: 2},
		},
	}
	tr := buildScopeTree(fe, &refCache{})
	if got := tr.Lookup(60, "m"); got != "path:./Main.java::Main.use.m" {
		t.Fatalf("use m=%q", got)
	}
	if got := tr.Lookup(200, "m"); got != "path:./Main.java::Main.main.m" {
		t.Fatalf("main m=%q", got)
	}
}

func TestScopeTree_TypeTypeNotOnClassTable(t *testing.T) {
	fe := &project.FileExtract{
		Path: "t.go",
		Scopes: []project.ScopeDef{
			{StartByte: 0, EndByte: 80, Parent: -1}, // Helper type
			{StartByte: 20, EndByte: 60, Parent: 0}, // Helper method
		},
		Atoms: []project.AtomDef{
			{Name: "Helper", StartByte: 5, EndByte: 11, ScopeIdx: 0},
			{Name: "Helper.Helper", StartByte: 30, EndByte: 36, ScopeIdx: 1},
		},
	}
	tr := buildScopeTree(fe, &refCache{})
	if got := tr.Lookup(25, "Helper"); got != "path:./t.go::Helper" {
		t.Fatalf("Helper in class=%q want type", got)
	}
	if got := tr.Lookup(0, "Helper"); got != "path:./t.go::Helper" {
		t.Fatalf("Helper at file=%q", got)
	}
}
