package pattern

import (
	"fmt"
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/tape"
)

func TestParse_InvertGroup(t *testing.T) {
	n, err := ParsePattern(`(seq (assert_not_behind (seq (capture NAME (regex "^Err")) "=" (capture PKG any) ".")) (capture E (ref "go:errors::New")) "(" (* any) ")")`)
	require.NoError(t, err)
	require.False(t, // Top-level is a seq: invert-group, then call sugar tokens for New(...).
		n.Kind != "seq" || len(n.Args) < 2,
		"want seq, got %s", mustJSON(n))

	g := n.Args[0]
	require.False(t, g.Kind != "group" || !g.Invert || g.As != "_",
		"invert group: %s", mustJSON(g))

	var sawRef bool
	for _, a := range n.Args[1:] {
		if a.Kind == "ref" && a.Ref == "go:errors::New" {
			sawRef = true
		}
	}
	require.True(t, sawRef,
		"missing New ref in %s", mustJSON(n))

	bare, err := ParsePattern(`(seq (assert_not_behind (token "x")) (token "y"))`)
	require.NoError(t, err)
	require.False(t, bare.Kind != "seq" || !bare.Args[0].Invert,
		"bare: %s", mustJSON(bare))

}

func TestMatch_InvertGroup_ErrorsNewNotAfterErrAssign(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import (
	"errors"
	"fmt"
)

var (
	ErrA = errors.New("a")
	ErrB = errors.New("b")
)

var ErrC = errors.New("c")

var notSentinel = errors.New("nope")

func local() error {
	err := errors.New("local")
	err = errors.New("assign")
	return errors.New("bare")
}

func nested() error {
	return fmt.Errorf("x: %w", errors.New("nested"))
}

func slice() []error {
	return []error{errors.New("s")}
}
`)
	path := lewpath.New(dir, "x.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example.com/t\n\ngo 1.22\n"), 0o644)
		require.NoError(t, err)
	}

	// Negative lookbehind: not immediately "Err* = <pkg> ." before New.
	// (Use capture PKG, not rest, so lookbehind cannot stretch from an earlier Err row.)
	pat, err := ParsePattern(`(seq (assert_not_behind (seq (capture NAME (regex "^Err")) "=" (capture PKG any) ".")) (capture E (ref "go:errors::New")) "(" (* any) ")")`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "x.go", src, pat)

	var got []string
	for _, m := range ms {
		got = append(got, m.Span.Text(src))
	}
	t.Logf("matches (%d): %v caps=%v", len(ms), got, capsOf(ms, src))

	// Allowed sentinel rows must NOT appear.
	for _, g := range got {
		if strings.Contains(g, `"a"`) || strings.Contains(g, `"b"`) || strings.Contains(g, `"c"`) {
			// only if it's the New call itself
			require.NotContains(t, []string{`errors.New("a")`, `errors.New("b")`, `errors.New("c")`}, g)

		}
	}

	wantSnips := []string{
		`errors.New("nope")`,
		`errors.New("local")`,
		`errors.New("assign")`,
		`errors.New("bare")`,
		`errors.New("nested")`,
		`errors.New("s")`,
	}
	for _, w := range wantSnips {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing violation %q; got %v", w, got)
		}
	}
	if len(ms) != len(wantSnips) {
		t.Errorf("match count=%d want %d", len(ms), len(wantSnips))
	}
}

func TestMatch_InvertGroup_AltArms(t *testing.T) {
	// Invert of (foo|bar) before baz: only plain baz matches.
	dir := t.TempDir()
	src := []byte("package p\n\nfunc f() {\n\t_ = foo baz\n\t_ = bar baz\n\t_ = qux baz\n}\n")
	// Use token stream: foo, baz etc as idents — no refs needed.
	path := lewpath.New(dir, "x.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(seq (assert_not_behind (alt (token "foo") (token "bar"))) (token "baz"))`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d want 1 (qux baz only); caps=%v text=%v", len(ms), capsOf(ms, src), func() []string {
			var s []string
			for _, m := range ms {
				s = append(s, m.Span.Text(src))
			}
			return s
		}())

	if got := ms[0].Span.Text(src); got != "baz" {
		// covering may expand — at least must include baz and not foo/bar line only
		require.Contains(t, got, "baz")
		require.NotContains(t, got, "foo")

	}
}

func TestLookbehindTokenConsumeBound(t *testing.T) {
	cases := []struct {
		sexp string
		want int
	}{
		{`(seq (assert_not_behind (token "pub")) (token "fn"))`, 1},
		{`(seq (assert_not_behind (alt (token "foo") (token "bar"))) (token "baz"))`, 1},
		{`(seq (assert_not_behind (seq (token "a") (token "b") (token "c") (token "d"))) (token "e"))`, 4},
		{`(seq (assert_not_behind (seq (capture NAME (regex "^Err")) "=" (capture PKG any) ".")) (token "x"))`, 4},
	}
	for _, tc := range cases {
		sub := lookbehindSubNFA(t, tc.sexp)
		if got := sub.tokenConsumeBound(); got != tc.want {
			t.Errorf("%s: tokenConsumeBound=%d want %d", tc.sexp, got, tc.want)
		}
	}
	// Consume cycle that can still accept: unbounded.
	cycle := &nfa{
		start:  0,
		accept: 1,
		states: []nfaState{
			{eps: []epsEdge{{to: 1}}, edges: []nfaEdge{{to: 0, pred: nfaPred{kind: predAny}}}},
			{},
		},
	}
	got := nfaMaxTokenConsume(cycle)
	require.Equal(t, -1, got,
		"cycle tokenConsumeBound=%d want -1", got)

}

func TestLookbehindStarNotRecognized(t *testing.T) {
	sub := lookbehindSubNFA(t, `(seq (assert_not_behind (seq "public" (* (not "class")))) (token "class"))`)
	require.True(t, sub.lookbehindStarNot,
		"want star-not lookbehind fast path")
	require.Equal(t, "public", sub.lookbehindStarNotFirst.text,
		"first=%q", sub.lookbehindStarNotFirst.text)

	bounded := lookbehindSubNFA(t, `(seq (assert_not_behind (token "pub")) (token "fn"))`)
	require.False(t, bounded.lookbehindStarNot,
		"token lookbehind must stay bounded, not star-not")

}

func TestLookbehindStarNotSameAsNFA(t *testing.T) {
	src := []byte("public class A {\n  public void foo() {}\n  void bar() {}\n  class Inner {}\n}\n")
	pf, err := ingestutil.ParseSource(t.Context(), ccgo.Engine{}, src, "A.java", "java")
	require.NoError(t, err)

	t.Cleanup(pf.Close)
	tokens := tokensFromTape(pf.Root, src, nil, tape.DefaultPolicy())
	require.NotEmpty(t, tokens,
		"no tokens")

	sub := lookbehindSubNFA(t, `(seq (assert_not_behind (seq "public" (* (not "(")))) (token "foo"))`)
	require.True(t, sub.lookbehindStarNot,
		"want star-not lookbehind")

	var look epsScratch
	for endPos := 0; endPos <= len(tokens); endPos++ {
		sub.lookbehindStarNot = false
		want := nfaMatchesEndingAt(sub, tokens, endPos, src, &look)
		sub.lookbehindStarNot = true
		got := nfaMatchesEndingAt(sub, tokens, endPos, src, &look)
		require.Equal(t, want, got,
			"endPos=%d fast=%v nfa=%v", endPos, got, want)

	}
}

func lookbehindSubNFA(t *testing.T, sexp string) *nfa {
	t.Helper()
	pat, err := ParsePattern(sexp)
	require.NoError(t, err)

	n, err := compilePattern(pat)
	require.NoError(t, err)

	for _, st := range n.states {
		for _, e := range st.eps {
			if e.negLookbehind != nil {
				return e.negLookbehind
			}
		}
	}
	require.FailNow(t, fmt.Sprintf("no lookbehind in %s", sexp))
	return nil
}
