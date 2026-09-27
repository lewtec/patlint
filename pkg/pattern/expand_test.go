package pattern

import (
	"reflect"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestExpandRefInCapture(t *testing.T) {
	got, err := ExpandSexp(`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) ")")`)
	require.NoError(t, err)

	want := []any{
		"seq",
		[]any{"capture", "F", []any{"ref", "go:fmt::Errorf"}},
		"(",
		[]any{"capture", "MSG", "any"},
		")",
	}
	require.True(t, reflect.DeepEqual(got, want),
		"got %#v\nwant %#v", got, want)

	// Compiles to Core
	pat, err := DecodePat(got)
	require.NoError(t, err)
	{

		_, err := CompilePat(pat)
		require.NoError(t, err)
	}

}

func TestExpandLetSequential(t *testing.T) {
	got, err := ExpandSexp(`(let a (token "x")
     b (seq a (token "y"))
  b)`)
	require.NoError(t, err)

	want := []any{"seq", []any{"token", "x"}, []any{"token", "y"}}
	require.True(t, reflect.DeepEqual(got, want),
		"got %#v want %#v", got, want)

}

func TestExpandCaptureNameNotSubstituted(t *testing.T) {
	// Binding named like a capture must not rewrite the capture name slot.
	got, err := ExpandSexp(`(let func (token "nope")
  (capture func (regex "^Test")))`)
	require.NoError(t, err)

	want := []any{"capture", "func", []any{"regex", "^Test"}}
	require.True(t, reflect.DeepEqual(got, want),
		"got %#v want %#v", got, want)

}

func TestExpandGetUnbound(t *testing.T) {
	_, err := ExpandSexp(`(get missing)`)
	require.Error(t, err,
		"want unbound error")

}

func TestExpandUnknownHead(t *testing.T) {
	_, err := ExpandSexp(`(not-a-head (token "x"))`)
	require.Error(t, err,
		"want unknown head error")

}

func TestExpandRepNumericUnrollsContiguous(t *testing.T) {
	got, err := ExpandSexp(`(rep 3 any)`)
	require.NoError(t, err)

	want := []any{"seq", "any", "any", "any"}
	require.True(t, reflect.DeepEqual(got, want),
		"got %#v\nwant %#v", got, want)

}

func TestExpandStarIsPrimitive(t *testing.T) {
	want := []any{"*", "any"}
	got, err := ExpandSexp(`(* any)`)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(got, want),
		"(* any) got %#v\nwant %#v", got, want)

}

func TestExpandRepStarForbidden(t *testing.T) {
	// Must not write (rep *|+|? body) — use (* body) / (+ body) / (? body).
	for _, form := range []string{
		"(rep " + "*" + " any)",
		"(rep + any)",
		"(rep ? any)",
	} {
		_, err := ExpandSexp(form)
		require.Error(t, err,
			"%s: want error", form)
		require.True(t, strings.Contains(err.Error(), "forbidden"),
			"%s: got %v", form, err)

	}
}

func TestExpandPreludeQuestionAndPlus(t *testing.T) {
	// Prelude (def ? …) / (def + …) — only (? p) / (+ p) heads, not (rep ? p).
	wantQ := []any{"alt", []any{"seq"}, []any{"token", "x"}}
	got, err := ExpandSexp(`(? (token "x"))`)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(got, wantQ),
		"(? …): got %#v\nwant %#v", got, wantQ)

	wantPlus := []any{"seq", "any", []any{"*", "any"}}
	got2, err := ExpandSexp(`(+ any)`)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(got2, wantPlus),
		"(+ any): got %#v\nwant %#v", got2, wantPlus)

	wantUntil := []any{"seq", []any{"*", []any{"not", "("}}, "("}
	got3, err := ExpandSexp(`(until "(")`)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(got3, wantUntil),
		"(until \"(\"): got %#v\nwant %#v", got3, wantUntil)

}

func TestExpandUserOverridesPrelude(t *testing.T) {
	// User let of ? wins over prelude.
	got, err := ExpandSexp(`(let ? (fn (p) (token "nope")) (? any))`)
	require.NoError(t, err)

	want := []any{"token", "nope"}
	require.True(t, reflect.DeepEqual(got, want),
		"got %#v want %#v", got, want)

}

func TestExpandRepRejectsTwoBounds(t *testing.T) {
	_, err := ExpandSexp(`(rep 2 3 any)`)
	require.Error(t, err,
		"want error for (rep N M body)")

}

func TestExpandFnApply(t *testing.T) {
	got, err := ExpandSexp(`(let under-test (fn (body)
      (under
        (seq (token "func") (capture func (regex "^Test")) "(" (* any) ")" "{" (* any) "}")
        body))
  (path "**/*_test.go" (under-test (seq (token "t") "." (token "Context")))))`)
	require.NoError(t, err)

	// path glob + under with region + body
	list, ok := got.([]any)
	require.False(t, !ok || list[0] != "path",
		"got %#v", got)
	require.Equal(t, "**/*_test.go", list[1],
		"glob %#v", list[1])

	under, ok := list[2].([]any)
	require.False(t, !ok || under[0] != "under",
		"under %#v", list[2])

	body, ok := under[2].([]any)
	require.False(t, !ok || body[0] != "seq",
		"body %#v", under[2])

}

func TestExpandDefProgn(t *testing.T) {
	got, err := ExpandSexp(`(progn
  (def wrap (x) (seq (token "A") x))
  (wrap (token "B")))`)
	require.NoError(t, err)

	want := []any{"seq", []any{"token", "A"}, []any{"token", "B"}}
	require.True(t, reflect.DeepEqual(got, want),
		"got %#v want %#v", got, want)

}

func TestExpandFnArity(t *testing.T) {
	_, err := ExpandSexp(`(let f (fn (a b) a) (f (token "x")))`)
	require.Error(t, err,
		"want arity error")

}

func TestExpandOsErrHelper(t *testing.T) {
	got, err := ExpandSexp(`(progn
  (def os-err-to-errors-is (old-fn err-ref)
    (under (lang go)
      (rewrite
        (seq (ref old-fn) "(" (token "err") ")")
        (seq (ref "go:errors::Is") "(" "err" ", " (ref err-ref) ")"))))
  (os-err-to-errors-is "go:os::IsExist" "go:io/fs::ErrExist"))`)
	require.NoError(t, err)

	// under (lang go) (rewrite (seq (ref IsExist) …) (seq (ref errors.Is) …))
	u, ok := got.([]any)
	require.False(t, !ok || u[0] != "under",
		"%#v", got)

	rw := u[2].([]any)
	require.Equal(t, "rewrite", rw[0],
		"rewrite %#v", rw)

	match := rw[1].([]any)
	require.Equal(t, "go:os::IsExist", match[1].([]any)[1],
		"old-fn %#v", match)

	emit := rw[2].([]any)
	require.Equal(t, "go:io/fs::ErrExist", emit[5].([]any)[1],
		"err-ref %#v", emit)

}

func TestExpandDefDocstringIgnored(t *testing.T) {
	got, err := ExpandSexp(`(progn
  (def wrap (x)
    "docs are ignored at expand"
    (seq (token "A") x))
  (wrap (token "B")))`)
	require.NoError(t, err)

	want := []any{"seq", []any{"token", "A"}, []any{"token", "B"}}
	require.True(t, reflect.DeepEqual(got, want),
		"got %#v want %#v", got, want)

}

func TestParseSexpRefCapture(t *testing.T) {
	pat, err := ParseSexp(`(seq (capture F (ref "go:fmt::Errorf")) "(" ")")`)
	require.NoError(t, err)

	seq, ok := pat.(Seq)
	require.False(t, !ok || len(seq.Items) < 2,
		"%#v", pat)

	cap0, ok := seq.Items[0].(Capture)
	require.True(t, ok,
		"%#v", seq.Items[0])
	{

		_, ok := cap0.Body.(Ref)
		require.True(t, ok,
			"want ref body, got %#v", cap0.Body)
	}

}

func TestParseMatcherLetPath(t *testing.T) {
	m, err := ParseMatcher(`(let g "**/*_test.go"
  (path g (token "interface{}")))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)
	require.True(t, cm.AcceptsFile("foo_test.go"),
		"want accept test file")
	require.False(t, cm.AcceptsFile("foo.go"),
		"want reject non-test")

}

func TestParseMatcherLetFnUnder(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p
func TestFoo(t *testing.T) { _ = t.Context() }
`)
	mustWrite(t, lewpath.New(dir, "x_test.go").String(), src)

	m, err := ParseMatcher(`(let under-test (fn (body)
      (under
        (seq (token "func") (capture func (regex "^Test")) "(" (* any) ")" "{" (* any) "}")
        body))
  (path "**/*_test.go"
    (under-test (seq (token "t") "." (token "Context")))))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	ms := mustMatchMatcher(t, dir, "x_test.go", src, cm)
	require.Len(t, ms, 1,
		"want 1 hit, got %d", len(ms))

	sp, ok := ms[0].CaptureFirst("func")
	require.True(t, ok,
		"missing func from region merge")
	require.Equal(t, "TestFoo", string(src[sp.StartByte:sp.EndByte]),
		"func=%q", src[sp.StartByte:sp.EndByte])

}

func TestExpandLetMissingBody(t *testing.T) {
	_, err := ExpandSexp(`(let a (token "x"))`)
	require.Error(t, err,
		"want error")

}

func TestDecodePatAnyRestKeywords(t *testing.T) {
	p, err := DecodePat([]any{"seq", "any", "rest"})
	require.NoError(t, err)

	seq := p.(Seq)
	{
		_, ok := seq.Items[0].(Any)
		require.True(t, ok,
			"%#v", seq.Items[0])
	}

	_, ok := seq.Items[1].(Rep)
	require.True(t, ok,
		"rest → rep, got %#v", seq.Items[1])

}
