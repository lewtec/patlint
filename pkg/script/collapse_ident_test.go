package script

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/internal/prelude"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

// Prove collapsed multi matches sequential MatchFileMatcher for sites+handlers
// on a synthetic tree with rewrites (prefer-any + return + failed-to Errorf).
func TestCollapsedVsSolo_SitesAndEdits(t *testing.T) {
	dir := t.TempDir()
	// Real rewrite targets: interface{}, return, and fmt.Errorf("failed to …", err).
	src := []byte(`package p

import "fmt"

func f(x interface{}) {}

func g() { return }

func wrap(err error) error {
	return fmt.Errorf("failed to open file: %w", err)
}
`)
	{
		err := os.WriteFile(lewpath.New(dir, "x.go").String(), src, 0o644)
		require.NoError(t, err)
	}

	// Mirrors janitor.rft go/failed-to-fmt-errorf (take MSG, re-quote slot).
	scriptSrc := `
(rule go/prefer-any
  warning
  "Prefer any"
  (under (lang go) (rewrite (token "interface{}") "any")))
(rule go/hit-return
  warning
  "return"
  (under (lang go) (token "return")))
(rule go/failed-to-fmt-errorf
  warning
  "Drop redundant failed to prefix in fmt.Errorf"
  (under (lang go)
    (rewrite
      (take "MSG"
        (seq
          (capture F (ref "go:fmt::Errorf"))
          "("
          (capture MSG (regex "(?i)^failed to\\s+(.*)" 1))
          ","
          (capture ERR any)
          ")"))
      (seq "\"" (slot) "\""))))
`
	prog, err := Load("t.rft", scriptSrc)
	require.NoError(t, err)
	require.True(t, prog.EnsurePlan().NeedLinks,
		"failed-to rule needs (ref …) links")

	// Spine path (collapsed multi-ε-NFA)
	resSpine, err := Run(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), prog, Options{Paths: []string{"."}})
	require.NoError(t, err)

	// Solo path: each matcher independently, same handlers
	resSolo, err := runSolo(t.Context(), dir, prog)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(normFindings(resSpine), normFindings(resSolo)),
		"findings differ\nspine=%#v\nsolo=%#v", normFindings(resSpine), normFindings(resSolo))
	require.True(t, reflect.DeepEqual(normApply(resSpine), normApply(resSolo)),
		"apply edits differ\nspine=%#v\nsolo=%#v", normApply(resSpine), normApply(resSolo))
	require.GreaterOrEqual(t, len(resSpine.ApplyEdits), 2,
		"expected rewrites for interface{} and failed-to Errorf; apply=%#v findings=%#v",
		normApply(resSpine), normFindings(resSpine))

	// failed-to should rewrite the MSG span (slot = capture group 1 text).
	var sawFailedTo bool
	for _, e := range resSpine.ApplyEdits {
		if strings.Contains(e.NewText, "open file") {
			sawFailedTo = true
			t.Logf("failed-to apply edit: start=%d end=%d new=%q", e.StartByte, e.EndByte, e.NewText)
		}
	}
	if !sawFailedTo {
		for _, f := range resSpine.Findings {
			if f.RuleID == "go/failed-to-fmt-errorf" {
				t.Logf("finding fixable=%v edits=%#v line=%d col=%d", f.Fixable, f.Edits, f.Line, f.Column)
			}
		}
		require.FailNow(t, fmt.Sprintf("expected failed-to fmt.Errorf rewrite; apply=%#v findings=%#v",
			normApply(resSpine), normFindings(resSpine)))
	}
}

func runSolo(ctx context.Context, root string, prog *Program) (Result, error) {
	// Reuse Run by forcing residual-only plan (no multi groups).
	old := prog.plan
	p := &Plan{NeedLinks: old.NeedLinks}
	for i, act := range prog.Actions {
		if act.Builtin != "" {
			p.Builtins = append(p.Builtins, i)
			continue
		}
		if act.Matcher != nil {
			p.Residual = append(p.Residual, i)
		}
	}
	prog.plan = p
	defer func() { prog.plan = old }()
	return Run(ctx, project.NewSession(root).WithEngine(treesitter.Engine{}), prog, Options{Paths: []string{"."}})
}

type nf struct {
	Rule, File, Msg   string
	Line, Col, EL, EC int
	Snippet           string
	Fixable, Skipped  bool
	Edits             []ne
}
type ne struct {
	File, New string
	S, E      uint32
}

func normFindings(r Result) []nf {
	var out []nf
	for _, f := range r.Findings {
		x := nf{Rule: f.RuleID, File: f.File, Msg: f.Message, Line: f.Line, Col: f.Column, EL: f.EndLine, EC: f.EndCol, Snippet: f.Snippet, Fixable: f.Fixable, Skipped: f.FixSkipped}
		for _, e := range f.Edits {
			x.Edits = append(x.Edits, ne{e.File, e.NewText, e.StartByte, e.EndByte})
		}
		out = append(out, x)
	}
	return out
}
func normApply(r Result) []ne {
	var out []ne
	for _, e := range r.ApplyEdits {
		out = append(out, ne{e.File, e.NewText, e.StartByte, e.EndByte})
	}
	return out
}

// Ensure plan actually used multi for this script.
func TestPlanUsesCollapsedMulti(t *testing.T) {
	prog, err := Load("t.rft", `
(rule a warning "A" (under (lang go) (token "x")))
(rule b warning "B" (under (lang go) (token "y")))
`)
	require.NoError(t, err)

	p := prog.EnsurePlan()
	require.False(t, len(p.Groups) != 1 || p.Groups[0].Multi == nil,
		"want collapsed multi: %+v", p)

}

// Shared under(region, body) with two body rules: spine under group == solo.
func TestCollapsedUnderVsSolo(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p
func helper(a int, b int, c int, d int) {
	var _ interface{}
	return
}
`)
	{
		err := os.WriteFile(lewpath.New(dir, "x.go").String(), src, 0o644)
		require.NoError(t, err)
	}

	// Same region for both; bodies differ. rewrite/take stay handlers.
	scriptSrc := `
(rule go/iface-in-func
  warning
  "iface"
  (under (lang go)
    (rewrite
      (under
        (seq (token "func") (capture name (regex "^[a-z]")) "(" (* any) ")" "{" (* any) "}")
        (token "interface{}"))
      "any")))
(rule go/return-in-func
  warning
  "ret"
  (under (lang go)
    (under
      (seq (token "func") (capture name (regex "^[a-z]")) "(" (* any) ")" "{" (* any) "}")
      (token "return"))))
`
	prog, err := Load("t.rft", scriptSrc)
	require.NoError(t, err)

	p := prog.EnsurePlan()
	require.False(t, len(p.Unders) != 1 || len(p.Unders[0].Arms) != 2,
		"want 1 under group with 2 arms: %s", p.PlanSummary())

	spine, err := Run(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), prog, Options{Paths: []string{"."}})
	require.NoError(t, err)

	solo, err := runSolo(t.Context(), dir, prog)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(normFindings(spine), normFindings(solo)),
		"under findings differ\nspine=%#v\nsolo=%#v", normFindings(spine), normFindings(solo))
	require.True(t, reflect.DeepEqual(normApply(spine), normApply(solo)),
		"under apply differ\nspine=%#v\nsolo=%#v", normApply(spine), normApply(solo))
	require.NotEmpty(t, spine.ApplyEdits,
		"want interface{} rewrite; findings=%#v", normFindings(spine))

}
