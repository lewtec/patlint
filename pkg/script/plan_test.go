package script

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, err)

	p := prog.EnsurePlan()
	require.NotNil(t, p,
		"nil plan")
	// a+b full-file; c flat under→body leaf → under group; d nested non-leaf body → residual
	require.Len(t, p.Groups, 1, "summary=%s", p.PlanSummary())
	require.Len(t, p.Groups[0].Arms, 2,
		"arms=%d want 2: %s", len(p.Groups[0].Arms), p.PlanSummary())
	require.False(t, len(p.Unders) != 1 || len(p.Unders[0].Arms) != 1,
		"under=%d arms=%v: %s", len(p.Unders), p.Unders, p.PlanSummary())
	require.Len(t, p.Residual, 1,
		"residual=%v want 1 nested under: %s", p.Residual, p.PlanSummary())

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
	require.NoError(t, err)

	p := prog.EnsurePlan()
	require.Len(t, p.Unders, 1,
		"under_groups=%d want 1: %s", len(p.Unders), p.PlanSummary())
	require.Len(t, p.Unders[0].Arms, 2,
		"under arms=%d want 2: %s", len(p.Unders[0].Arms), p.PlanSummary())
	require.False(t, p.Unders[0].Multi == nil || p.Unders[0].Region == nil,
		"want region+multi: %#v", p.Unders[0])

}

func TestFormatPlan_ShowsMultiPrimitive(t *testing.T) {
	script := `
(rule a warning "A"
  (under (lang go) (rewrite (token "interface{}") "any")))
(rule b warning "B"
  (under (lang go) (rewrite (token "return") "return")))
`
	prog, err := Load("t.rft", script)
	require.NoError(t, err)

	dump := FormatPlan(prog, false, "")
	require.True(t, strings.Contains(dump, "MatchFileMultiLeaves"),
		"want multi primitive name: %s", dump)
	require.True(t, strings.Contains(dump, "spine group[0]"),
		"want spine group: %s", dump)
	require.True(t, strings.Contains(dump, "MultiLeafNFA collapsed"),
		"want collapsed multi dump: %s", dump)
	require.True(t, strings.Contains(dump, "tag id="),
		"want tagged accepts/handlers: %s", dump)

	// filter one arm (rule id "a")
	one := FormatPlan(prog, false, "a")
	require.Equal(t, 1, strings.Count(one, "### arm action["),
		"want single arm for filter a: %s", one)

}

func TestRun_SpineSameAsPreferAny(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc f(x interface{}) {}\nfunc g() { return }\n")
	{
		err := os.WriteFile(lewpath.New(dir, "x.go").String(), src, 0o644)
		require.NoError(t, err)
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
	require.NoError(t, err)

	sum := prog.EnsurePlan().PlanSummary()
	require.True(t, strings.Contains(sum, "arms=2"),
		"want fused arms: %s", sum)

	res, err := Run(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), prog, Options{Paths: []string{"."}})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(res.Findings), 2,
		"findings=%d want >=2", len(res.Findings))

}
