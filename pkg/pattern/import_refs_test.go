package pattern_test

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestEmitRefs(t *testing.T) {
	emit, err := pattern.ParseEmit(`(ref "go:context::Background")`)
	require.NoError(t, err)

	refs := pattern.EmitRefs(emit)
	require.False(t, len(refs) != 1 || !strings.Contains(refs[0], "context"),
		"refs=%v", refs)

}

func TestWithImportHygiene_AddsFmt(t *testing.T) {
	dir := t.TempDir()
	// File has no fmt import; rewrite will emit fmt.Errorf via (ref …).
	src := []byte("package p\n\nfunc f() error {\n\treturn nil\n}\n")
	path := lewpath.New(dir, "p.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	op, err := pattern.OpFromCLI("rewrite", "go",
		`(seq (token "return") (token "nil"))`,
		`(seq "return " (ref "go:fmt::Errorf") "(\"x\")")`,
	)
	require.NoError(t, err)

	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	res, err := w.Run(t.Context(), op, pattern.RunOptions{Paths: []string{path}})
	require.NoError(t, err)
	require.NotEmpty(t, res.Edits,
		"expected edits")
	{

		err := project.ApplyEdits(t.Context(), dir, res.Edits)
		require.NoError(t, err)
	}

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	text := string(got)
	require.True(t, strings.Contains(text, "fmt.Errorf"),
		"missing fmt.Errorf:\n%s", text)
	require.True(t, strings.Contains(text, `"fmt"`),
		"missing import fmt:\n%s", text)

}

func TestImportNeedsForRule(t *testing.T) {
	rule, err := pattern.RuleFromStrings(`(token "x")`, `(ref "go:net/http::Get")`)
	require.NoError(t, err)

	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	needs := pattern.ImportNeedsForRule(vm, "go", rule)
	require.False(t, len(needs) != 1 || needs[0].ImportPath != "net/http",
		"needs=%+v", needs)

	py := pattern.ImportNeedsForRule(vm, "python", rule)
	require.Empty(t, py,
		"python must not import go refs: %+v", py)

}

func TestWithImportHygiene_PrunesNamedAfterSiteRewrite(t *testing.T) {
	dir := t.TempDir()
	// context only used at the rewritten site; after rewrite it is unused and pruned.
	src := []byte("package p\n\nimport (\n\t\"context\"\n\t\"fmt\"\n)\n\nfunc f() {\n\t_ = context.Background()\n\tfmt.Println()\n}\n")
	path := lewpath.New(dir, "p.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	op, err := pattern.OpFromCLI("rewrite", "go",
		`(seq (token "context") "." (token "Background") "(" ")")`,
		`nil`,
	)
	require.NoError(t, err)

	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	res, err := w.Run(t.Context(), op, pattern.RunOptions{Paths: []string{path}})
	require.NoError(t, err)
	{

		err := project.ApplyEdits(t.Context(), dir, res.Edits)
		require.NoError(t, err)
	}

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	text := string(got)
	require.False(t, strings.Contains(text, `"context"`),
		"expected context import pruned:\n%s", text)
	require.True(t, strings.Contains(text, `"fmt"`),
		"fmt must remain:\n%s", text)
	require.True(t, strings.Contains(text, "nil"),
		"rewrite missing:\n%s", text)

}
