package ingest_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
	"github.com/lewtec/patlint/pkg/tape"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"
)

func parseGo(t *testing.T, src string) (*sitter.Node, []byte, func()) {
	t.Helper()
	pf, err := ingestutil.ParseSource(t.Context(), treesitter.Engine{}, []byte(src), "x.go", "go")
	require.NoError(t, err)

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

	sess := project.NewSession(".").WithEngine(treesitter.Engine{})
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	bindings := ingest.LocalBindingsForLanguage(t.Context(), vm, sess, root, source, "go", "x.go")
	w, err := walker.NewWalker(t.Context(), sess, vm)
	require.NoError(t, err)

	cells, _, err := w.BuildTape(t.Context(), source, "x.go")
	require.NoError(t, err)

	// Project whole file; look for the if/return shape with holes
	terms := ingest.ProjectAbstract(cells, source, "x.go", bindings, ingest.DefaultAbstractOptions())
	got := ingest.FormatTerms(terms)
	// Expect %r1 / %r2 for a,b and structural if/>/return
	require.Contains(t, got, "if")
	require.Contains(t, got, ">")
	require.Contains(t, got, "return")
	require.Contains(t, got, "%r1")
	require.Contains(t, got, "%r2")
	require.NotContains(t, got, " a ")
	require.NotContains(t, got, " b ")

}

func TestProjectAbstract_LitKeepsValue(t *testing.T) {
	src := "package p\nfunc F() string { return \"hi\" }\n"
	_, source, cleanup := parseGo(t, src)
	defer cleanup()
	sess := project.NewSession(".").WithEngine(treesitter.Engine{})
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), sess, vm)
	require.NoError(t, err)

	cells, _, err := w.BuildTape(t.Context(), source, "x.go")
	require.NoError(t, err)

	terms := ingest.ProjectAbstract(cells, source, "x.go", nil, ingest.DefaultAbstractOptions())
	got := ingest.FormatTerms(terms)
	require.Contains(t, got, `LIT("hi")`)

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
	require.Equal(t, []string{"@go:fmt::Println"}, terms)

}

func TestFormatTermsSexp(t *testing.T) {
	got := ingest.FormatTermsSexp([]string{
		"{", "if", "%r1", ">", "%r2", "{", "return", "%r1", "}", "return", "%r2", "}",
	})
	want := `(seq "{" "if" (unify r1 any) ">" (unify r2 any) "{" "return" (unify r1 any) "}" "return" (unify r2 any) "}")`
	require.Equal(t, want, got)

	got2 := ingest.FormatTermsSexp([]string{"LIT(42)", "@go:fmt::Println", "%r3"})
	require.Contains(t, got2, `"42"`)
	require.Contains(t, got2, `(ref "go:fmt::Println")`)
	require.Contains(t, got2, `(unify r3 any)`)

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
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	units, err := w.AbstractFile(t.Context(), root, source, "x.go", true)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(units), 2)

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
	require.NotEmpty(t, maxT, "units=%v", names(units))
	require.NotEmpty(t, minT, "units=%v", names(units))
	require.NotEqual(t, maxT, minT, "Max and Min should differ with numbered holes")
	// Both should share the comparison skeleton
	require.Contains(t, maxT, "if")
	require.Contains(t, minT, "if")

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
