package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/store"
)

func TestRuleFromStrings_Take(t *testing.T) {
	rule, err := pattern.RuleFromStrings(`(take "c" (capture c (ref "go:context::Background")))`, `(ref "go:testing::T")`)
	require.NoError(t, err)
	require.Equal(t, "c", rule.Take,
		"Take=%q", rule.Take)
	require.NotNil(t, rule.Emit,
		"empty emit")
	require.NotNil(t, rule.Matcher,
		"RuleFromStrings should keep compiled Matcher")

}

func TestRuleFromOp_KeepsMatcher(t *testing.T) {
	op, err := pattern.OpFromCLI("rewrite", "go", `(token "interface{}")`, `any`)
	require.NoError(t, err)

	rule, err := pattern.RuleFromOp(op)
	require.NoError(t, err)
	require.NotNil(t, rule.Matcher,
		"RuleFromOp should attach Matcher")

}

func TestRefLeafRule_ExpandFile(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import "fmt"

func f() {
	fmt.Println("x")
}
`)
	path := lewpath.New(dir, "p.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	sess := project.NewSession(".").WithEngine(ccgo.Engine{})
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	fe, err := ingest.CollectExtracts(t.Context(), ingest.ExtractSource{
		Kind:    ingest.ExtractHop,
		Root:    dir,
		Paths:   []string{path},
		Session: sess,
		Policy:  vm,
	})
	require.NoError(t, err)

	st := store.New()
	ingest.Ingest(st, fe, vm)
	result, err := ingest.EvalStore(t.Context(), dir, st, vm)
	require.NoError(t, err)

	pf, err := ingestutil.ParseSourceFile(t.Context(), ccgo.Engine{}, path, "go")
	require.NoError(t, err)

	defer pf.Close()

	target := ""
	for _, u := range result.Uses {
		if u.Target != "" {
			target = u.Target
			break
		}
	}
	require.NotEmpty(t, target,
		"no uses in result; atoms=%d uses=%d", len(result.Atoms), len(result.Uses))

	rule, err := pattern.RefLeafRule(target, "Renamed")
	require.NoError(t, err)
	require.True(t, rule.NeedsLinks(),
		"RefLeafRule should need links")

	matches, edits, err := rule.ExpandFile(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), dir, "p.go", src, pf.Root, result)
	require.NoError(t, err)
	require.NotEmpty(t, matches,
		"expected matches for target %q", target)
	require.NotEmpty(t, edits,
		"expected edits")

	for _, e := range edits {
		require.Equal(t, "p.go", e.File,
			"edit file=%q", e.File)
		require.Equal(t, "Renamed", e.NewText,
			"NewText=%q", e.NewText)
		require.Less(t, e.StartByte, e.EndByte,
			"empty span: %+v", e)

	}
}

func TestRuleFromOp_Rewrite(t *testing.T) {
	op, err := pattern.OpFromCLI("rewrite", "go", `(token "interface{}")`, `any`)
	require.NoError(t, err)

	rule, err := pattern.RuleFromOp(op)
	require.NoError(t, err)
	{

		err := rule.Valid()
		require.NoError(t, err)
	}

}
