package script

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestLoadAndRunPreferAny(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc f(x interface{}) {}\n")
	{
		err := os.WriteFile(lewpath.New(dir, "x.go").String(), src, 0o644)
		require.NoError(t, err)
	}

	script := `
(rule go/prefer-any
  warning
  "Prefer any over interface{} (Go 1.18+)"
  (under (lang go)
    (rewrite
      (token "interface{}")
      "any")))
`
	prog, err := Load("test.rft", script)
	require.NoError(t, err)
	require.Len(t, prog.Actions, 1,
		"actions %d", len(prog.Actions))
	require.False(t, prog.Actions[0].Report == nil || prog.Actions[0].Report.ID != "go/prefer-any",
		"%+v", prog.Actions[0].Report)
	require.NotNil(t, prog.Actions[0].Emit,
		"want emit")

	res, err := Run(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), prog, Options{Paths: []string{"."}})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(res.Findings), 1,
		"want findings, got %d", len(res.Findings))
	require.True(t, res.Findings[0].Fixable,
		"want fixable")
	require.GreaterOrEqual(t, len(res.ApplyEdits), 1,
		"want apply edits")

}

func TestLoadDefAndCall(t *testing.T) {
	script := `
(def os-err-to-errors-is (old-fn err-ref)
  (under (lang go)
    (rewrite
      (seq (ref old-fn) "(" (token "err") ")")
      (seq (ref "go:errors::Is") "(" "err" ", " (ref err-ref) ")"))))

(rule go/os-isexist-err
  warning
  "See docs"
  (os-err-to-errors-is "go:os::IsExist" "go:io/fs::ErrExist"))
`
	prog, err := Load("t.rft", script)
	require.NoError(t, err)
	require.Len(t, prog.Actions, 1,
		"actions %#v", prog.Actions)
	require.Equal(t, "go", prog.Actions[0].Lang,
		"lang %q", prog.Actions[0].Lang)
	require.NotNil(t, prog.Actions[0].Emit,
		"emit")

}

func TestNestedRuleRejectedClearly(t *testing.T) {
	// Nested (rule …) must not become a silent match node or opaque "unknown head".
	cases := []string{
		`(under (lang go) (rule go/x warning "m" (token "y")))`,
		`(def wrap () (rule go/x warning "m" (token "y")))
(wrap)`,
	}
	for _, src := range cases {
		_, err := Load("bad.rft", src)
		require.Error(t, err,
			"want error for nested rule, src=%q", src)
		require.True(t, strings.Contains(err.Error(), "top level"),
			"want clear top-level message, got %v", err)
		require.False(t, strings.Contains(err.Error(), `unknown head "rule"`),
			"still opaque unknown head: %v", err)

	}
}

func TestTopLevelRewriteNoReport(t *testing.T) {
	// rewrite without rule: edits only, no findings
	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) {}\n")
	{
		err := os.WriteFile(lewpath.New(dir, "x.go").String(), src, 0o644)
		require.NoError(t, err)
	}

	script := `
(rewrite
  (under (lang go) (token "interface{}"))
  "any")
`
	prog, err := Load("t.rft", script)
	require.NoError(t, err)

	res, err := Run(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), prog, Options{})
	require.NoError(t, err)
	require.Empty(t, res.Findings,
		"want no lint findings, got %d", len(res.Findings))
	require.GreaterOrEqual(t, len(res.ApplyEdits), 1,
		"want edits from bare rewrite")

	_ = strings.Contains
}
