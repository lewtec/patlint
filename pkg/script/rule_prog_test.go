package script

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/datalog"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
	"github.com/lewtec/patlint/pkg/store"
)

func TestRuleClauses(t *testing.T) {
	prog, err := Load("t.rft", `
(rule go/prefer-any
  warning
  "Prefer any"
  (under (lang go)
    (rewrite (token "interface{}") "any")))
`)
	require.NoError(t, err)

	cl := prog.RuleClauses("x.go")
	require.Len(t, cl, 1)
	require.Equal(t, store.RelationFinding, cl[0].Head.Rel)
	require.Len(t, cl[0].Body, 1)
	require.Equal(t, "$rule", cl[0].Body[0].Rel)

	st := store.New()
	h := &ruleHost{prog: prog, sites: map[int][]pattern.Match{
		0: {{Span: ingestutil.Span{StartByte: 10, EndByte: 22}}},
	}}
	datalog.Eval(t.Context(), st, cl, h)
	require.True(t, st.Contains(store.RelationFinding, store.Tuple{"x.go", "10", "22", "go/prefer-any", "warning", "Prefer any"}),
		"findings=%v", st.Rows(store.RelationFinding))

}

func TestRunRuleWritesFinding(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc f() {\n\treturn\n}\n")
	require.NoError(t, os.WriteFile(lewpath.New(dir, "x.go").String(), src, 0o644))

	prog, err := Load("t.rft", `
(rule go/hit-return
  warning
  "return"
  (under (lang go) (token "return")))
`)
	require.NoError(t, err)

	res, err := Run(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), prog, Options{Paths: []string{"."}})
	require.NoError(t, err)
	require.NotEmpty(t, res.Findings, "want finding from Datalog rule")
	require.Equal(t, "go/hit-return", res.Findings[0].RuleID)

}
