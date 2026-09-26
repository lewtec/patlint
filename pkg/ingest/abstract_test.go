package ingest_test

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/tape"
	"github.com/lewtec/patlint/pkg/walker"
)

func parseGo(t *testing.T, src string) (*sitter.Node, []byte, func()) {
	t.Helper()
	pf, err := ingestutil.ParseSource(t.Context(), ccgo.Engine{}, []byte(src), "x.go", "go")
	if err != nil {
		t.Fatal(err)
	}
	return pf.Root, pf.Source, pf.Close
}

func TestProjectAbstract_MaxShape(t *testing.T) {
	src := `package p
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
`
	root, source, cleanup := parseGo(t, src)
	defer cleanup()

	sess := project.NewSession(".").WithEngine(ccgo.Engine{})
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	bindings := ingest.LocalBindingsForLanguage(t.Context(), vm, sess, root, source, "go", "x.go")
	w, err := walker.NewWalker(t.Context(), sess, vm)
	if err != nil {
		t.Fatal(err)
	}
	cells, _, err := w.BuildTape(t.Context(), source, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	// Project whole file; look for the if/return shape with holes
	terms := ingest.ProjectAbstract(cells, source, "x.go", bindings, ingest.DefaultAbstractOptions())
	got := ingest.FormatTerms(terms)
	// Expect %r1 / %r2 for a,b and structural if/>/return
	if !strings.Contains(got, "if") || !strings.Contains(got, ">") || !strings.Contains(got, "return") {
		t.Fatalf("missing control tokens: %s", got)
	}
	if !strings.Contains(got, "%r1") || !strings.Contains(got, "%r2") {
		t.Fatalf("expected numbered holes, got: %s", got)
	}
	if strings.Contains(got, " a ") || strings.Contains(got, " b ") {
		t.Fatalf("surface names should be holes: %s", got)
	}
}

func TestProjectAbstract_LitKeepsValue(t *testing.T) {
	src := "package p\nfunc F() string { return \"hi\" }\n"
	_, source, cleanup := parseGo(t, src)
	defer cleanup()
	sess := project.NewSession(".").WithEngine(ccgo.Engine{})
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), sess, vm)
	if err != nil {
		t.Fatal(err)
	}
	cells, _, err := w.BuildTape(t.Context(), source, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	terms := ingest.ProjectAbstract(cells, source, "x.go", nil, ingest.DefaultAbstractOptions())
	got := ingest.FormatTerms(terms)
	if !strings.Contains(got, `LIT("hi")`) {
		t.Fatalf("want LIT(\"hi\") in %s", got)
	}
}

func TestProjectAbstract_ProductRef(t *testing.T) {
	// Synthetic cell with Target (as tape.Build attaches from Uses).
	cells := []tape.Cell{{
		Span:   tape.Span{StartByte: 0, EndByte: 5},
		Type:   "identifier",
		Target: "go:fmt::Println",
	}}
	src := []byte("xxxxx")
	terms := ingest.ProjectAbstract(cells, src, "x.go", nil, ingest.DefaultAbstractOptions())
	if len(terms) != 1 || terms[0] != "@go:fmt::Println" {
		t.Fatalf("terms=%v", terms)
	}
}

func TestFormatTermsSexp(t *testing.T) {
	got := ingest.FormatTermsSexp([]string{
		"{", "if", "%r1", ">", "%r2", "{", "return", "%r1", "}", "return", "%r2", "}",
	})
	want := `(seq "{" "if" (unify r1 any) ">" (unify r2 any) "{" "return" (unify r1 any) "}" "return" (unify r2 any) "}")`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
	got2 := ingest.FormatTermsSexp([]string{"LIT(42)", "@go:fmt::Println", "%r3"})
	if !strings.Contains(got2, `"42"`) || !strings.Contains(got2, `(ref "go:fmt::Println")`) || !strings.Contains(got2, `(unify r3 any)`) {
		t.Fatalf("got2=%s", got2)
	}
}

func TestAbstractFile_MaxMinDiffer(t *testing.T) {
	src := `package p
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func Min(a, b int) int {
	if a > b {
		return b
	}
	return a
}
`
	root, source, cleanup := parseGo(t, src)
	defer cleanup()
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	units, err := w.AbstractFile(t.Context(), root, source, "x.go", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 2 {
		t.Fatalf("units=%d want >=2", len(units))
	}
	var maxT, minT string
	for _, u := range units {
		s := ingest.FormatTerms(u.Terms)
		if u.Name == "Max" {
			maxT = s
		}
		if u.Name == "Min" {
			minT = s
		}
	}
	if maxT == "" || minT == "" {
		t.Fatalf("max=%q min=%q units=%v", maxT, minT, names(units))
	}
	if maxT == minT {
		t.Fatalf("Max and Min should differ with numbered holes:\nMax %s\nMin %s", maxT, minT)
	}
	// Both should share the comparison skeleton
	if !strings.Contains(maxT, "if") || !strings.Contains(minT, "if") {
		t.Fatalf("both need if: max=%s min=%s", maxT, minT)
	}
	t.Logf("Max: %s", maxT)
	t.Logf("Min: %s", minT)
}

func names(u []ingest.AbstractUnit) []string {
	var o []string
	for _, x := range u {
		o = append(o, x.Name)
	}
	return o
}
