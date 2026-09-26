package pattern

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestRewriteTakeOnly(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import (
	"context"
	"testing"
)

func TestFoo(t *testing.T) {
	_ = context.Background()
	_ = context.Background()
}

func Helper() {
	_ = context.Background()
}
`)
	path := lewpath.New(dir, "x_test.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	// Without *: only first Background in TestFoo.
	op, err := OpFromCLI("rewrite", "go",
		`(take "c" (seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (capture c (ref "go:context::Background")) (* any) "}"))`,
		`t.Context`,
	)
	require.NoError(t, err)
	require.Equal(t, "c", op.Take,
		"Take=%q want c", op.Take)

	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	res, err := applyOp(t.Context(), sess, vm, op, RunOptions{})
	require.NoError(t, err)
	require.Len(t, res.Edits, 1,
		"edits=%d want 1 (first only); edits=%v", len(res.Edits), res.Edits)

	out, err := os.ReadFile(path)
	require.NoError(t, err)

	got := string(out)
	require.Equal(t, 1, strings.Count(got, "t.Context()"),
		"t.Context count=%d want 1\n%s", strings.Count(got, "t.Context()"), got)
	require.Equal(t, 2, strings.Count(got, "context.Background"),
		"context.Background count=%d want 2 (second Test + Helper)\n%s",
		strings.Count(got, "context.Background"), got)

}

func TestRewriteTakeMultiStar(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import (
	"context"
	"testing"
)

func TestFoo(t *testing.T) {
	_ = context.Background()
	_ = context.Background()
}

func Helper() {
	_ = context.Background()
}
`)
	path := lewpath.New(dir, "x_test.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	// With *: every Background inside each Test* function.
	op, err := OpFromCLI("rewrite", "go",
		`(take "c" (seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (* (capture c (ref "go:context::Background"))) (* any) "}"))`,
		`t.Context`,
	)
	require.NoError(t, err)

	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	res, err := applyOp(t.Context(), sess, vm, op, RunOptions{})
	require.NoError(t, err)
	require.Len(t, res.Edits, 2,
		"edits=%d want 2; edits=%v", len(res.Edits), res.Edits)

	out, err := os.ReadFile(path)
	require.NoError(t, err)

	got := string(out)
	require.Equal(t, 2, strings.Count(got, "t.Context()"),
		"t.Context count=%d want 2\n%s", strings.Count(got, "t.Context()"), got)
	require.Equal(t, 1, strings.Count(got, "context.Background"),
		"context.Background count=%d want 1 (Helper only)\n%s",
		strings.Count(got, "context.Background"), got)

}

func TestParseMultiStar(t *testing.T) {
	n, err := ParsePattern(`(* (capture c (ref "go:context::Background")))`)
	require.NoError(t, err)
	require.False(t, n.Kind != "ref" || !n.Multi || n.MultiPlus || n.As != "c",
		"got %+v", n)

	n, err = ParsePattern(`(* (capture c (group (ref "go:context::Background"))))`)
	require.NoError(t, err)
	require.False(t, n.Kind != "group" || !n.Multi || n.MultiPlus,
		"group multi got %+v", n)

	n, err = ParsePattern(`(* (capture c (group (seq (token "t") "." (token "Context")))))`)
	require.NoError(t, err)
	require.False(t, n.Kind != "group" || !n.Multi || n.MultiPlus || n.As != "c",
		"t.Context multi group %+v", n)

	n, err = ParsePattern(`(+ (capture c (ref "go:context::Background")))`)
	require.NoError(t, err)
	require.False(t, // (+) expands; accept Rep+ or Seq of first+rest.
		n.Kind != "seq" && !(n.Multi && n.MultiPlus),
		"plus got %+v", n)

}

func TestMultiPlusRequiresOneSite(t *testing.T) {
	dir := t.TempDir()
	// No context.Background — * would still match the Test envelope; + must not.
	src := []byte(`package p

import "testing"

func TestFoo(t *testing.T) {
	_ = 1
}
`)
	path := lewpath.New(dir, "x_test.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	star, err := ParsePattern(`(seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (* (capture c (ref "go:context::Background"))) (* any) "}")`)
	require.NoError(t, err)

	plus, err := ParsePattern(`(seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (+ (capture c (ref "go:context::Background"))) (* any) "}")`)
	require.NoError(t, err)

	msStar := mustMatchFile(t, dir, path, "x_test.go", src, star)
	require.Len(t, msStar, 1,
		"* matches=%d want 1 (zero multi still matches envelope)", len(msStar))

	msPlus := mustMatchFile(t, dir, path, "x_test.go", src, plus)
	require.Empty(t, msPlus,
		"+ matches=%d want 0 when no Background", len(msPlus))

	src2 := []byte(`package p

import (
	"context"
	"testing"
)

func TestFoo(t *testing.T) {
	_ = context.Background()
}
`)
	path2 := lewpath.New(dir, "y_test.go").String()
	{
		err := os.WriteFile(path2, src2, 0o644)
		require.NoError(t, err)
	}

	msPlus = mustMatchFile(t, dir, path2, "y_test.go", src2, plus)
	require.Len(t, msPlus, 1,
		"+ with Background matches=%d want 1", len(msPlus))

}

func TestRewriteMultiGroupTContext(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import "testing"

func TestFoo(t *testing.T) {
	_ = t.Context()
	_ = t.Context()
}

func Helper(t *testing.T) {
	_ = t.Context()
}
`)
	path := lewpath.New(dir, "x_test.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	op, err := OpFromCLI("rewrite", "go",
		`(take "c" (seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (* (capture c (group (seq (token "t") "." (token "Context"))))) (* any) "}"))`,
		`(ref "go:context::Background")`,
	)
	require.NoError(t, err)

	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	res, err := applyOp(t.Context(), sess, vm, op, RunOptions{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, // 2 site leaf rewrites + import "context" ensure for @go:context::Background
		len(res.Edits), 2,
		"edits=%d want >=2; %v", len(res.Edits), res.Edits)

	out, err := os.ReadFile(path)
	require.NoError(t, err)

	got := string(out)
	require.Equal(t, 2, strings.Count(got, "context.Background()"),
		"context.Background count=%d\n%s", strings.Count(got, "context.Background()"), got)
	require.Equal(t, 1, strings.Count(got, "t.Context()"),
		"Helper should keep one t.Context:\n%s", got)
	require.True(t, strings.Contains(got, `"context"`),
		"expected import context:\n%s", got)

}

func TestRewriteNestedMultiIteration(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import "testing"

func TestFoo(t *testing.T) {
	_ = t.Context()
	_ = t.Context()
}
`)
	path := lewpath.New(dir, "x_test.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	op, err := OpFromCLI("rewrite", "go",
		`(take "c" (seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (* (capture i (group (seq (capture c (group (seq (token "t") "." (token "Context")))) (* any))))) "}"))`,
		`(ref "go:context::Background")`,
	)
	require.NoError(t, err)

	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	res, err := applyOp(t.Context(), sess, vm, op, RunOptions{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, // 2 site rewrites + import ensure for @go:context::Background
		len(res.Edits), 2,
		"edits=%d want >=2; %v", len(res.Edits), res.Edits)

	out, err := os.ReadFile(path)
	require.NoError(t, err)

	got := string(out)
	require.Equal(t, 2, strings.Count(got, "context.Background()"),
		"context.Background count=%d\n%s", strings.Count(got, "context.Background()"), got)
	require.True(t, strings.Contains(got, `"context"`),
		"expected import context:\n%s", got)

}
