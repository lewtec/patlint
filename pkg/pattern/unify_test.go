package pattern

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestParseUnifyAndDiscipline(t *testing.T) {
	n, err := ParsePattern(`(seq (unify a any) ">" (unify b any) ":" (unify a any) "?" (unify b any))`)
	require.NoError(t, err)

	// seq or similar with unify captures
	var foundA, foundB bool
	var walk func(Node)
	walk = func(n Node) {
		if n.As == "a" {
			foundA = true
			if !n.Unify {
				t.Error("a should Unify")
			}
		}
		if n.As == "b" {
			foundB = true
			if !n.Unify {
				t.Error("b should Unify")
			}
		}
		if n.Callee != nil {
			walk(*n.Callee)
		}
		for _, a := range n.Args {
			walk(a)
		}
	}
	walk(n)
	require.False(t, !foundA || !foundB,
		"missing captures a=%v b=%v ir=%+v", foundA, foundB, n)

	if _, err := ParsePattern(`(seq (capture a any) "+" (unify a any))`); err == nil {
		require.FailNow(t, "expected mix same name error")
	} else {
		msg := err.Error()
		require.False(t, !strings.Contains(msg, "both") && !strings.Contains(msg, "discipline"),
			"err=%v", err)
	}

	// (* (unify …)) is legal Core.
	if n, err := ParsePattern(`(* (unify a (ref "go:fmt::Errorf")))`); err != nil {
		require.NoError(t, err)
	} else {
		require.False(t, !n.Multi || !n.Unify,
			"want unify multi, got %+v", n)
	}
	{

		_, err := ParsePattern(`(seq (unify a any) "!=" (token "nil") "?" (unify a any) ":" (capture fallback any))`)
		require.NoError(t, err)
	}

}

func TestUnifyMatchEqualAndReject(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func other(x, y int) int {
	if x > y {
		return y
	}
	return x
}
`)
	path := lewpath.New(dir, "m.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(seq (token "if") (unify a any) ">" (unify b any) "{" (token "return") (unify a any) "}")`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "m.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d want 1 (only max); caps=%v", len(ms), capsOf(ms, src))
	{

		got := ms[0].Captures["a"][0].Text(src)
		require.Equal(t, "x", got,
			"a=%q", got)
	}

	got := ms[0].Captures["b"][0].Text(src)
	require.Equal(t, "y", got,
		"b=%q", got)

}

func TestAppendDoubleCapture(t *testing.T) {
	dir := t.TempDir()
	// Use single-token '+' (tree-sitter keeps '==' as one token; pattern '==' is two '=' lits).
	src := []byte("package p\n\nfunc f(x, y int) int { return x + y }\n")
	path := lewpath.New(dir, "m.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	// two (capture a …): append both sites (no unify)
	pat, err := ParsePattern(`(seq (capture a any) "+" (capture a any))`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "m.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d caps=%v", len(ms), capsOf(ms, src))
	require.Len(t, ms[0].Captures["a"], 2,
		"append sites=%d want 2; caps=%v", len(ms[0].Captures["a"]), PublicCaptures(ms[0], src))
	// (capture a) + (capture a) with x + y still matches (no equality) — both sites stored
	require.Equal(t, "x", ms[0].Captures["a"][0].Text(src))
	require.Equal(t, "y", ms[0].Captures["a"][1].Text(src))

}

func TestUnifyRejectsDifferent(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc f(x, y int) int { return x + y }\n")
	path := lewpath.New(dir, "m.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(seq (unify a any) "+" (unify a any))`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "m.go", src, pat)
	require.Empty(t, ms,
		"want 0 matches for x+y with unify, got %d caps=%v", len(ms), capsOf(ms, src))

	src2 := []byte("package p\n\nfunc f(x int) int { return x + x }\n")
	path2 := lewpath.New(dir, "m2.go").String()
	{
		err := os.WriteFile(path2, src2, 0o644)
		require.NoError(t, err)
	}

	ms = mustMatchFile(t, dir, path2, "m2.go", src2, pat)
	require.Len(t, ms, 1,
		"want 1 for x+x, got %d", len(ms))

}

func TestRewriteUnifyTemplate(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

func f(x, y int) int {
	if x > y {
		return x
	}
	return y
}
`)
	path := lewpath.New(dir, "m.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(seq (token "if") (unify a any) ">" (unify b any) "{" (token "return") (unify a any) "}")`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "m.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d", len(ms))
	require.False(t, ms[0].Captures["a"][0].Text(src) != "x" || ms[0].Captures["b"][0].Text(src) != "y",
		"caps=%v", PublicCaptures(ms[0], src))

}

func TestMixUnifyAndAppend(t *testing.T) {
	dir := t.TempDir()
	// Avoid multi-char ops (!=); use > 0 for single-token '>'.
	src := []byte(`package p

func f(x, y int) int {
	if x > 0 {
		return x
	}
	return y
}
`)
	path := lewpath.New(dir, "m.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(seq (token "if") (unify a any) ">" "0" "{" (token "return") (unify a any) "}" (* any) (token "return") (capture fallback any))`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "m.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d caps=%v", len(ms), capsOf(ms, src))
	require.Equal(t, "x", ms[0].Captures["a"][0].Text(src),
		"a=%v", PublicCaptures(ms[0], src))
	require.Equal(t, "y", ms[0].Captures["fallback"][0].Text(src),
		"fallback=%v", PublicCaptures(ms[0], src))

}
