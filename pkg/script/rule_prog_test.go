package script

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/datalog"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
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
	if err != nil {
		t.Fatal(err)
	}
	cl := prog.RuleClauses("x.go")
	if len(cl) != 1 {
		t.Fatalf("clauses=%d", len(cl))
	}
	if cl[0].Head.Rel != store.RelationFinding {
		t.Fatalf("head=%s", cl[0].Head.Rel)
	}
	if len(cl[0].Body) != 1 || cl[0].Body[0].Rel != "$rule" {
		t.Fatalf("body=%+v", cl[0].Body)
	}

	st := store.New()
	h := &ruleHost{prog: prog, sites: map[int][]pattern.Match{
		0: {{Span: ingestutil.Span{StartByte: 10, EndByte: 22}}},
	}}
	datalog.Eval(t.Context(), st, cl, h)
	if !st.Contains(store.RelationFinding, store.Tuple{"x.go", "10", "22", "go/prefer-any", "warning", "Prefer any"}) {
		t.Fatalf("findings=%v", st.Rows(store.RelationFinding))
	}
}

func TestRunRuleWritesFinding(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc f() {\n\treturn\n}\n")
	if err := os.WriteFile(lewpath.New(dir, "x.go").String(), src, 0o644); err != nil {
		t.Fatal(err)
	}
	prog, err := Load("t.rft", `
(rule go/hit-return
  warning
  "return"
  (under (lang go) (token "return")))
`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), prog, Options{Paths: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) < 1 {
		t.Fatalf("want finding from Datalog rule, got %d", len(res.Findings))
	}
	if res.Findings[0].RuleID != "go/hit-return" {
		t.Fatalf("%+v", res.Findings[0])
	}
}
