package pattern

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/tape"
)

func TestParse_InvertGroup(t *testing.T) {
	n, err := ParsePattern(`(seq (assert_not_behind (seq (capture NAME (regex "^Err")) "=" (capture PKG any) ".")) (capture E (ref "go:errors::New")) "(" (* any) ")")`)
	if err != nil {
		t.Fatal(err)
	}
	// Top-level is a seq: invert-group, then call sugar tokens for New(...).
	if n.Kind != "seq" || len(n.Args) < 2 {
		t.Fatalf("want seq, got %s", mustJSON(n))
	}
	g := n.Args[0]
	if g.Kind != "group" || !g.Invert || g.As != "_" {
		t.Fatalf("invert group: %s", mustJSON(g))
	}
	var sawRef bool
	for _, a := range n.Args[1:] {
		if a.Kind == "ref" && a.Ref == "go:errors::New" {
			sawRef = true
		}
	}
	if !sawRef {
		t.Fatalf("missing New ref in %s", mustJSON(n))
	}
	bare, err := ParsePattern(`(seq (assert_not_behind (token "x")) (token "y"))`)
	if err != nil {
		t.Fatal(err)
	}
	if bare.Kind != "seq" || !bare.Args[0].Invert {
		t.Fatalf("bare: %s", mustJSON(bare))
	}
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
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example.com/t\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Negative lookbehind: not immediately "Err* = <pkg> ." before New.
	// (Use capture PKG, not rest, so lookbehind cannot stretch from an earlier Err row.)
	pat, err := ParsePattern(`(seq (assert_not_behind (seq (capture NAME (regex "^Err")) "=" (capture PKG any) ".")) (capture E (ref "go:errors::New")) "(" (* any) ")")`)
	if err != nil {
		t.Fatal(err)
	}
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
			if g == `errors.New("a")` || g == `errors.New("b")` || g == `errors.New("c")` {
				t.Fatalf("sentinel New should be excluded: %q in %v", g, got)
			}
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
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	pat, err := ParsePattern(`(seq (assert_not_behind (alt (token "foo") (token "bar"))) (token "baz"))`)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	if len(ms) != 1 {
		t.Fatalf("matches=%d want 1 (qux baz only); caps=%v text=%v", len(ms), capsOf(ms, src), func() []string {
			var s []string
			for _, m := range ms {
				s = append(s, m.Span.Text(src))
			}
			return s
		}())
	}
	if got := ms[0].Span.Text(src); got != "baz" {
		// covering may expand — at least must include baz and not foo/bar line only
		if !strings.Contains(got, "baz") || strings.Contains(got, "foo") {
			t.Fatalf("span=%q", got)
		}
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
	if got := nfaMaxTokenConsume(cycle); got != -1 {
		t.Fatalf("cycle tokenConsumeBound=%d want -1", got)
	}
}

func TestLookbehindStarNotRecognized(t *testing.T) {
	sub := lookbehindSubNFA(t, `(seq (assert_not_behind (seq "public" (* (not "class")))) (token "class"))`)
	if !sub.lookbehindStarNot {
		t.Fatal("want star-not lookbehind fast path")
	}
	if sub.lookbehindStarNotFirst.text != "public" {
		t.Fatalf("first=%q", sub.lookbehindStarNotFirst.text)
	}
	bounded := lookbehindSubNFA(t, `(seq (assert_not_behind (token "pub")) (token "fn"))`)
	if bounded.lookbehindStarNot {
		t.Fatal("token lookbehind must stay bounded, not star-not")
	}
}

func TestLookbehindStarNotSameAsNFA(t *testing.T) {
	src := []byte("public class A {\n  public void foo() {}\n  void bar() {}\n  class Inner {}\n}\n")
	pf, err := ingestutil.ParseSource(t.Context(), ccgo.Engine{}, src, "A.java", "java")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pf.Close)
	tokens := tokensFromTape(pf.Root, src, nil, tape.DefaultPolicy())
	if len(tokens) == 0 {
		t.Fatal("no tokens")
	}
	sub := lookbehindSubNFA(t, `(seq (assert_not_behind (seq "public" (* (not "(")))) (token "foo"))`)
	if !sub.lookbehindStarNot {
		t.Fatal("want star-not lookbehind")
	}
	var look epsScratch
	for endPos := 0; endPos <= len(tokens); endPos++ {
		sub.lookbehindStarNot = false
		want := nfaMatchesEndingAt(sub, tokens, endPos, src, &look)
		sub.lookbehindStarNot = true
		got := nfaMatchesEndingAt(sub, tokens, endPos, src, &look)
		if got != want {
			t.Fatalf("endPos=%d fast=%v nfa=%v", endPos, got, want)
		}
	}
}

func lookbehindSubNFA(t *testing.T, sexp string) *nfa {
	t.Helper()
	pat, err := ParsePattern(sexp)
	if err != nil {
		t.Fatal(err)
	}
	n, err := compilePattern(pat)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range n.states {
		for _, e := range st.eps {
			if e.negLookbehind != nil {
				return e.negLookbehind
			}
		}
	}
	t.Fatalf("no lookbehind in %s", sexp)
	return nil
}
