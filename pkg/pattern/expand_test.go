package pattern

import (
	"reflect"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestExpandRefInCapture(t *testing.T) {
	got, err := ExpandSexp(`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) ")")`)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{
		"seq",
		[]any{"capture", "F", []any{"ref", "go:fmt::Errorf"}},
		"(",
		[]any{"capture", "MSG", "any"},
		")",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
	// Compiles to Core
	pat, err := DecodePat(got)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompilePat(pat); err != nil {
		t.Fatal(err)
	}
}

func TestExpandLetSequential(t *testing.T) {
	got, err := ExpandSexp(`(let a (token "x")
     b (seq a (token "y"))
  b)`)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"seq", []any{"token", "x"}, []any{"token", "y"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestExpandCaptureNameNotSubstituted(t *testing.T) {
	// Binding named like a capture must not rewrite the capture name slot.
	got, err := ExpandSexp(`(let func (token "nope")
  (capture func (regex "^Test")))`)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"capture", "func", []any{"regex", "^Test"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestExpandGetUnbound(t *testing.T) {
	_, err := ExpandSexp(`(get missing)`)
	if err == nil {
		t.Fatal("want unbound error")
	}
}

func TestExpandUnknownHead(t *testing.T) {
	_, err := ExpandSexp(`(not-a-head (token "x"))`)
	if err == nil {
		t.Fatal("want unknown head error")
	}
}

func TestExpandRepNumericUnrollsContiguous(t *testing.T) {
	got, err := ExpandSexp(`(rep 3 any)`)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"seq", "any", "any", "any"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestExpandStarIsPrimitive(t *testing.T) {
	want := []any{"*", "any"}
	got, err := ExpandSexp(`(* any)`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("(* any) got %#v\nwant %#v", got, want)
	}
}

func TestExpandRepStarForbidden(t *testing.T) {
	// Must not write (rep *|+|? body) — use (* body) / (+ body) / (? body).
	for _, form := range []string{
		"(rep " + "*" + " any)",
		"(rep + any)",
		"(rep ? any)",
	} {
		_, err := ExpandSexp(form)
		if err == nil {
			t.Fatalf("%s: want error", form)
		}
		if !strings.Contains(err.Error(), "forbidden") {
			t.Fatalf("%s: got %v", form, err)
		}
	}
}

func TestExpandPreludeQuestionAndPlus(t *testing.T) {
	// Prelude (def ? …) / (def + …) — only (? p) / (+ p) heads, not (rep ? p).
	wantQ := []any{"alt", []any{"seq"}, []any{"token", "x"}}
	got, err := ExpandSexp(`(? (token "x"))`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantQ) {
		t.Fatalf("(? …): got %#v\nwant %#v", got, wantQ)
	}

	wantPlus := []any{"seq", "any", []any{"*", "any"}}
	got2, err := ExpandSexp(`(+ any)`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got2, wantPlus) {
		t.Fatalf("(+ any): got %#v\nwant %#v", got2, wantPlus)
	}

	wantUntil := []any{"seq", []any{"*", []any{"not", "("}}, "("}
	got3, err := ExpandSexp(`(until "(")`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got3, wantUntil) {
		t.Fatalf("(until \"(\"): got %#v\nwant %#v", got3, wantUntil)
	}
}

func TestExpandUserOverridesPrelude(t *testing.T) {
	// User let of ? wins over prelude.
	got, err := ExpandSexp(`(let ? (fn (p) (token "nope")) (? any))`)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"token", "nope"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestExpandRepRejectsTwoBounds(t *testing.T) {
	_, err := ExpandSexp(`(rep 2 3 any)`)
	if err == nil {
		t.Fatal("want error for (rep N M body)")
	}
}

func TestExpandFnApply(t *testing.T) {
	got, err := ExpandSexp(`(let under-test (fn (body)
      (under
        (seq (token "func") (capture func (regex "^Test")) "(" (* any) ")" "{" (* any) "}")
        body))
  (path "**/*_test.go" (under-test (seq (token "t") "." (token "Context")))))`)
	if err != nil {
		t.Fatal(err)
	}
	// path glob + under with region + body
	list, ok := got.([]any)
	if !ok || list[0] != "path" {
		t.Fatalf("got %#v", got)
	}
	if list[1] != "**/*_test.go" {
		t.Fatalf("glob %#v", list[1])
	}
	under, ok := list[2].([]any)
	if !ok || under[0] != "under" {
		t.Fatalf("under %#v", list[2])
	}
	body, ok := under[2].([]any)
	if !ok || body[0] != "seq" {
		t.Fatalf("body %#v", under[2])
	}
}

func TestExpandDefProgn(t *testing.T) {
	got, err := ExpandSexp(`(progn
  (def wrap (x) (seq (token "A") x))
  (wrap (token "B")))`)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"seq", []any{"token", "A"}, []any{"token", "B"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestExpandFnArity(t *testing.T) {
	_, err := ExpandSexp(`(let f (fn (a b) a) (f (token "x")))`)
	if err == nil {
		t.Fatal("want arity error")
	}
}

func TestExpandOsErrHelper(t *testing.T) {
	got, err := ExpandSexp(`(progn
  (def os-err-to-errors-is (old-fn err-ref)
    (under (lang go)
      (rewrite
        (seq (ref old-fn) "(" (token "err") ")")
        (seq (ref "go:errors::Is") "(" "err" ", " (ref err-ref) ")"))))
  (os-err-to-errors-is "go:os::IsExist" "go:io/fs::ErrExist"))`)
	if err != nil {
		t.Fatal(err)
	}
	// under (lang go) (rewrite (seq (ref IsExist) …) (seq (ref errors.Is) …))
	u, ok := got.([]any)
	if !ok || u[0] != "under" {
		t.Fatalf("%#v", got)
	}
	rw := u[2].([]any)
	if rw[0] != "rewrite" {
		t.Fatalf("rewrite %#v", rw)
	}
	match := rw[1].([]any)
	if match[1].([]any)[1] != "go:os::IsExist" {
		t.Fatalf("old-fn %#v", match)
	}
	emit := rw[2].([]any)
	if emit[5].([]any)[1] != "go:io/fs::ErrExist" {
		t.Fatalf("err-ref %#v", emit)
	}
}

func TestExpandDefDocstringIgnored(t *testing.T) {
	got, err := ExpandSexp(`(progn
  (def wrap (x)
    "docs are ignored at expand"
    (seq (token "A") x))
  (wrap (token "B")))`)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"seq", []any{"token", "A"}, []any{"token", "B"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestParseSexpRefCapture(t *testing.T) {
	pat, err := ParseSexp(`(seq (capture F (ref "go:fmt::Errorf")) "(" ")")`)
	if err != nil {
		t.Fatal(err)
	}
	seq, ok := pat.(Seq)
	if !ok || len(seq.Items) < 2 {
		t.Fatalf("%#v", pat)
	}
	cap0, ok := seq.Items[0].(Capture)
	if !ok {
		t.Fatalf("%#v", seq.Items[0])
	}
	if _, ok := cap0.Body.(Ref); !ok {
		t.Fatalf("want ref body, got %#v", cap0.Body)
	}
}

func TestParseMatcherLetPath(t *testing.T) {
	m, err := ParseMatcher(`(let g "**/*_test.go"
  (path g (token "interface{}")))`)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	if !cm.AcceptsFile("foo_test.go") {
		t.Fatal("want accept test file")
	}
	if cm.AcceptsFile("foo.go") {
		t.Fatal("want reject non-test")
	}
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
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchMatcher(t, dir, "x_test.go", src, cm)
	if len(ms) != 1 {
		t.Fatalf("want 1 hit, got %d", len(ms))
	}
	sp, ok := ms[0].CaptureFirst("func")
	if !ok {
		t.Fatal("missing func from region merge")
	}
	if string(src[sp.StartByte:sp.EndByte]) != "TestFoo" {
		t.Fatalf("func=%q", src[sp.StartByte:sp.EndByte])
	}
}

func TestExpandLetMissingBody(t *testing.T) {
	_, err := ExpandSexp(`(let a (token "x"))`)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestDecodePatAnyRestKeywords(t *testing.T) {
	p, err := DecodePat([]any{"seq", "any", "rest"})
	if err != nil {
		t.Fatal(err)
	}
	seq := p.(Seq)
	if _, ok := seq.Items[0].(Any); !ok {
		t.Fatalf("%#v", seq.Items[0])
	}
	if _, ok := seq.Items[1].(Rep); !ok {
		t.Fatalf("rest → rep, got %#v", seq.Items[1])
	}
}
