package datalog

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/pkg/store"
	"github.com/stretchr/testify/require"
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
	require.True(t, s.Contains("path", store.Tuple{"a", "c"}),
		"expected path a c, rows=%v", s.Rows("path"))
	require.True(t, s.Contains("marked", store.Tuple{"a"}),
		"expected marked a")

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
	s.Insert(store.RelationImport, store.Tuple{
		"main.go", "_", "image/png", "", "100", "120", "0", "0", "1", "108", "118", "0",
	})
	Eval(t.Context(), s, QueryProgram(), StoreHost{Store: s})
	require.True(t, s.Contains(store.RelationFinding, store.Tuple{"main.go", "12", "24", DeadImportID, DeadImportLevel, DeadImportMsg}),
		"want unused fmt, findings=%v used=%v", s.Rows(store.RelationFinding), s.Rows(store.RelationUsedName))
	require.True(t, s.Contains(store.RelationEdit, store.Tuple{"main.go", "12", "24", ""}),
		"want delete edit, edits=%v", s.Rows(store.RelationEdit))

	for _, row := range s.Rows(store.RelationFinding) {
		if len(row) > 2 {
			require.NotEqual(t, "25", row[1], "os is used: %v", row)
			require.NotEqual(t, "100", row[1], "blank local is not a named import: %v", row)
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
	require.True(t, s.Contains(store.RelationEdit, store.Tuple{"main.go", "5", "11", "doHelp"}),
		"want def edit, edits=%v", s.Rows(store.RelationEdit))
	require.True(t, s.Contains(store.RelationEdit, store.Tuple{"main.go", "40", "46", "doHelp"}),
		"want use edit, edits=%v", s.Rows(store.RelationEdit))

	for _, row := range s.Rows(store.RelationEdit) {
		if len(row) > 2 {
			require.NotEqual(t, "20", row[1], "alias/via-import must not edit: %v", row)
			require.NotEqual(t, "0", row[1], "alias/via-import must not edit: %v", row)
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
	require.True(t, s.Contains(store.RelationEdit, store.Tuple{"src.go", "14", "30", ""}),
		"want hole, edits=%v", s.Rows(store.RelationEdit))
	require.True(t, s.Contains(store.RelationEdit, store.Tuple{"dst.go", "20", "20", "func helper() {\n}\n"}),
		"want place, edits=%v", s.Rows(store.RelationEdit))
	require.True(t, s.Contains(store.RelationEdit, store.Tuple{"./main.go", "10", "16", "dst.go"}),
		"want import rewrite, edits=%v", s.Rows(store.RelationEdit))

	for _, row := range s.Rows(store.RelationEdit) {
		if len(row) > 1 && (row[0] == "./src.go" || row[0] == "src.go") {
			require.NotEqual(t, "10", row[1], "must not rewrite src import: %v", row)
		}
	}
}
