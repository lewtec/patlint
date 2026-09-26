package script

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestCompilePlan_FusesFullFileLeaves(t *testing.T) {
	script := `
(rule a warning "A"
  (under (lang go) (rewrite (token "interface{}") "any")))
(rule b warning "B"
  (under (lang go) (rewrite (token "return") "return")))
(rule c warning "C"
  (under (lang go)
    (under (seq (token "func") (capture f any) "(" (* any) ")" "{" (* any) "}")
      (token "x"))))
(rule d warning "D"
  (under (lang go)
    (under (seq (token "func") (capture f any) "(" (* any) ")" "{" (* any) "}")
      (under (token "x") (token "y")))))
`
	prog, err := Load("t.rft", script)
	if err != nil {
		t.Fatal(err)
	}
	p := prog.EnsurePlan()
	if p == nil {
		t.Fatal("nil plan")
	}
	// a+b full-file; c flat under→body leaf → under group; d nested non-leaf body → residual
	if len(p.Groups) != 1 {
		t.Fatalf("groups=%d summary=%s", len(p.Groups), p.PlanSummary())
	}
	if len(p.Groups[0].Arms) != 2 {
		t.Fatalf("arms=%d want 2: %s", len(p.Groups[0].Arms), p.PlanSummary())
	}
	if len(p.Unders) != 1 || len(p.Unders[0].Arms) != 1 {
		t.Fatalf("under=%d arms=%v: %s", len(p.Unders), p.Unders, p.PlanSummary())
	}
	if len(p.Residual) != 1 {
		t.Fatalf("residual=%v want 1 nested under: %s", p.Residual, p.PlanSummary())
	}
}

func TestCompilePlan_FusesUnderBodies(t *testing.T) {
	// Two rules: same region (func…), different leaf bodies → one under group.
	script := `
(rule a warning "A"
  (under (lang go)
    (under (seq (token "func") (capture n any) "(" (* any) ")" "{" (* any) "}")
      (token "interface{}"))))
(rule b warning "B"
  (under (lang go)
    (under (seq (token "func") (capture n any) "(" (* any) ")" "{" (* any) "}")
      (token "return"))))
`
	// Note: nested under → residual today. Flat under(regionLeaf, bodyLeaf):
	script = `
(rule a warning "A"
  (under (lang go)
    (rewrite
      (under
        (seq (token "func") (capture n (regex "^[a-z]")) "(" (* any) ")" "{" (* any) "}")
        (token "interface{}"))
      "any")))
(rule b warning "B"
  (under (lang go)
    (under
      (seq (token "func") (capture n (regex "^[a-z]")) "(" (* any) ")" "{" (* any) "}")
      (token "return"))))
`
	prog, err := Load("t.rft", script)
	if err != nil {
		t.Fatal(err)
	}
	p := prog.EnsurePlan()
	if len(p.Unders) != 1 {
		t.Fatalf("under_groups=%d want 1: %s", len(p.Unders), p.PlanSummary())
	}
	if len(p.Unders[0].Arms) != 2 {
		t.Fatalf("under arms=%d want 2: %s", len(p.Unders[0].Arms), p.PlanSummary())
	}
	if p.Unders[0].Multi == nil || p.Unders[0].Region == nil {
		t.Fatalf("want region+multi: %#v", p.Unders[0])
	}
}

func TestFormatPlan_ShowsMultiPrimitive(t *testing.T) {
	script := `
(rule a warning "A"
  (under (lang go) (rewrite (token "interface{}") "any")))
(rule b warning "B"
  (under (lang go) (rewrite (token "return") "return")))
`
	prog, err := Load("t.rft", script)
	if err != nil {
		t.Fatal(err)
	}
	dump := FormatPlan(prog, false, "")
	if !strings.Contains(dump, "MatchFileMultiLeaves") {
		t.Fatalf("want multi primitive name: %s", dump)
	}
	if !strings.Contains(dump, "spine group[0]") {
		t.Fatalf("want spine group: %s", dump)
	}
	if !strings.Contains(dump, "MultiLeafNFA collapsed") {
		t.Fatalf("want collapsed multi dump: %s", dump)
	}
	if !strings.Contains(dump, "tag id=") {
		t.Fatalf("want tagged accepts/handlers: %s", dump)
	}
	// filter one arm (rule id "a")
	one := FormatPlan(prog, false, "a")
	if strings.Count(one, "### arm action[") != 1 {
		t.Fatalf("want single arm for filter a: %s", one)
	}
}

func TestRun_SpineSameAsPreferAny(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc f(x interface{}) {}\nfunc g() { return }\n")
	if err := os.WriteFile(lewpath.New(dir, "x.go").String(), src, 0o644); err != nil {
		t.Fatal(err)
	}
	script := `
(rule go/prefer-any
  warning
  "Prefer any"
  (under (lang go) (rewrite (token "interface{}") "any")))
(rule go/hit-return
  warning
  "return"
  (under (lang go) (token "return")))
`
	prog, err := Load("t.rft", script)
	if err != nil {
		t.Fatal(err)
	}
	sum := prog.EnsurePlan().PlanSummary()
	if !strings.Contains(sum, "arms=2") {
		t.Fatalf("want fused arms: %s", sum)
	}
	res, err := Run(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), prog, Options{Paths: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) < 2 {
		t.Fatalf("findings=%d want >=2", len(res.Findings))
	}
}
