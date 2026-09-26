package datalog

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/pkg/store"
)

func TestEvalJoinAndNegation(t *testing.T) {
	s := store.New()
	s.Insert("edge", store.Tuple{"a", "b"})
	s.Insert("edge", store.Tuple{"b", "c"})
	s.Insert("keep", store.Tuple{"a"})
	p := Program{
		cl(l("path", "?x", "?y"), l("edge", "?x", "?y")),
		cl(l("path", "?x", "?z"), l("path", "?x", "?y"), l("edge", "?y", "?z")),
		cl(l("marked", "?x"), l("keep", "?x"), Lit{Rel: "edge", Args: []Arg{Var("x"), Var("x")}, Neg: true}),
	}
	Eval(t.Context(), s, p, nil)
	if !s.Contains("path", store.Tuple{"a", "c"}) {
		t.Fatalf("expected path a c, rows=%v", s.Rows("path"))
	}
	if !s.Contains("marked", store.Tuple{"a"}) {
		t.Fatal("expected marked a")
	}
}

func TestQueryDeadImports(t *testing.T) {
	s := store.New()
	s.Insert(store.RelationImport, store.Tuple{
		"main.go", "", "fmt", "", "12", "24", "0", "0", "0", "19", "24", "0",
	})
	s.Insert(store.RelationImport, store.Tuple{
		"main.go", "", "os", "", "25", "37", "0", "0", "0", "32", "37", "0",
	})
	// os is used in the body; fmt is only named at the import site (inside span).
	s.Append(store.RelationUse, store.Tuple{"main.go", "fmt", "19", "24", "", "-1", "0", "0"})
	s.Append(store.RelationUse, store.Tuple{"main.go", "os", "50", "52", "", "-1", "0", "1"})
	s.Insert(store.RelationImport, store.Tuple{
		"main.go", "*", "star", "*", "80", "90", "0", "0", "0", "0", "0", "1",
	})
	Eval(t.Context(), s, QueryProgram(), StoreHost{Store: s})

	if !s.Contains(store.RelationFinding, store.Tuple{"main.go", "12", "24", DeadImportID, DeadImportLevel, DeadImportMsg}) {
		t.Fatalf("want unused fmt, findings=%v used=%v", s.Rows(store.RelationFinding), s.Rows(store.RelationUsedName))
	}
	if !s.Contains(store.RelationEdit, store.Tuple{"main.go", "12", "24", ""}) {
		t.Fatalf("want delete edit, edits=%v", s.Rows(store.RelationEdit))
	}
	for _, row := range s.Rows(store.RelationFinding) {
		if len(row) > 2 && row[1] == "25" {
			t.Fatalf("os is used: %v", row)
		}
	}
}

func TestRenameProgram(t *testing.T) {
	s := store.New()
	s.Insert(store.RelationAtom, store.Tuple{"main.go", "helper", "5", "11", "0", "-1"})
	s.Insert(store.RelationAtomInFile, store.Tuple{"main.go", "helper", "path:./main.go::helper"})
	s.Append(store.RelationBinds, store.Tuple{"main.go", "40", "46", "path:./main.go", "path:./main.go::helper", "0"})
	s.Append(store.RelationBinds, store.Tuple{"main.go", "20", "26", "path:./main.go", "path:./main.go::helper", "1"})
	s.Insert(store.RelationAlias, store.Tuple{"path:./main.go", "0", "0", "path:./main.go::helper", "", "0", "0"})
	host := StoreHost{
		Store: s,
		RenameLeaf: func(ref string) (string, bool) {
			if ref == "path:./main.go::helper" {
				return "doHelp", true
			}
			return "", false
		},
	}
	Eval(t.Context(), s, RenameProgram(), host)
	if !s.Contains(store.RelationEdit, store.Tuple{"main.go", "5", "11", "doHelp"}) {
		t.Fatalf("want def edit, edits=%v", s.Rows(store.RelationEdit))
	}
	if !s.Contains(store.RelationEdit, store.Tuple{"main.go", "40", "46", "doHelp"}) {
		t.Fatalf("want use edit, edits=%v", s.Rows(store.RelationEdit))
	}
	for _, row := range s.Rows(store.RelationEdit) {
		if len(row) > 2 && (row[1] == "20" || row[1] == "0") {
			t.Fatalf("alias/via-import must not edit: %v", row)
		}
	}
}

func TestMoveProgram(t *testing.T) {
	s := store.New()
	s.Insert(store.RelationAlias, store.Tuple{"path:./main.go", "8", "12", "path:./src.go::helper", "src.go", "10", "16"})
	s.Insert(store.RelationAlias, store.Tuple{"path:./src.go", "8", "12", "path:./src.go::helper", "src.go", "10", "16"})
	host := StoreHost{
		Store:     s,
		MoveHole:  [][]string{{"src.go", "14", "30"}},
		MovePlace: [][]string{{"dst.go", "20", "20", "func helper() {\n}\n"}},
		RewriteSpec: func(file, spec string) (string, bool) {
			if spec == "src.go" {
				return "dst.go", true
			}
			return "", false
		},
		NotMoveFile: func(file string) bool {
			f := strings.TrimPrefix(file, "./")
			return f != "src.go" && f != "dst.go"
		},
	}
	Eval(t.Context(), s, MoveProgram(), host)
	if !s.Contains(store.RelationEdit, store.Tuple{"src.go", "14", "30", ""}) {
		t.Fatalf("want hole, edits=%v", s.Rows(store.RelationEdit))
	}
	if !s.Contains(store.RelationEdit, store.Tuple{"dst.go", "20", "20", "func helper() {\n}\n"}) {
		t.Fatalf("want place, edits=%v", s.Rows(store.RelationEdit))
	}
	if !s.Contains(store.RelationEdit, store.Tuple{"./main.go", "10", "16", "dst.go"}) {
		t.Fatalf("want import rewrite, edits=%v", s.Rows(store.RelationEdit))
	}
	for _, row := range s.Rows(store.RelationEdit) {
		if len(row) > 0 && (row[0] == "./src.go" || row[0] == "src.go") && row[1] == "10" {
			t.Fatalf("must not rewrite src import: %v", row)
		}
	}
}
