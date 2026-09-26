package ingest_test

import (
	"os"
	"sort"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

// Pack-only Go extract via Session view + Bind claims (internal/prelude/language_go.rft).
//
//	go test ./pkg/ingest/ -run TestGoPackExtract_SessionSpine -v
func TestGoPackExtract_SessionSpine(t *testing.T) {
	dir := t.TempDir()
	src := `package demo

import (
	"fmt"
	aliased "strings"
)

type T struct {
	X int
}

func Hello(x int) int {
	return x
}

func (t *T) Method() {
	fmt.Println(t.X)
	_ = aliased.ToUpper("a")
}
`
	if err := os.WriteFile(lewpath.New(dir, "demo.go").String(), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var fe *project.FileExtract
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WalkExtracts(t.Context(), ingest.SourceHop(dir, lewpath.New(dir, "demo.go").String()), func(x *project.FileExtract) bool {
		fe = x
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if fe == nil {
		t.Fatal("no extract")
	}
	if fe.Language != "go" {
		t.Fatalf("language=%q", fe.Language)
	}
	if fe.Package != "demo" {
		t.Fatalf("package=%q want demo", fe.Package)
	}

	atomNames := map[string]bool{}
	for _, a := range fe.Atoms {
		atomNames[a.Name] = true
		t.Logf("atom name=%q", a.Name)
	}
	want := []string{"Hello", "T", "T.Method", "T.X"}
	for _, w := range want {
		if !atomNames[w] {
			t.Errorf("missing atom %q; have %v", w, sortedKeys(atomNames))
		}
	}
	// type_spec must not spray field/type tokens as atoms
	for _, bad := range []string{"struct", "int", "X", "x", "fmt", "aliased"} {
		if atomNames[bad] {
			t.Errorf("spurious atom %q", bad)
		}
	}
	if len(fe.Imports) < 2 {
		t.Errorf("want >=2 imports, got %+v", fe.Imports)
	}
	var sawFmtPath bool
	for _, im := range fe.Imports {
		if im.SourcePath == "fmt" && im.PathEndByte > im.PathStartByte {
			sawFmtPath = true
		}
	}
	if !sawFmtPath {
		t.Error("fmt as-import missing path token span")
	}

	res, err := w.Load(t.Context(), ingest.SourceProject(dir), ingest.MaterializeOptions{ExpandImports: true})
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]bool{}
	for _, a := range res.Atoms {
		refs[a.Reference] = true
	}
	for _, w := range []string{
		"path:./demo.go::Hello",
		"path:./demo.go::T",
		"path:./demo.go::T.Method",
		"path:./demo.go::T.X",
	} {
		if !refs[w] {
			t.Errorf("missing ref %s; have %v", w, sortedKeys(refs))
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
