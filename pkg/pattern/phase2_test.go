package pattern

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestParseOptionalQuant(t *testing.T) {
	core, err := ParseToPat(`(? (capture c (ref "go:context::Background")))`)
	if err != nil {
		t.Fatal(err)
	}
	// Expand turns (?) into (alt (seq) body).
	alt, ok := core.(Alt)
	if !ok || len(alt.Items) != 2 {
		t.Fatalf("want expanded optional Alt, got %#v", core)
	}
}

func TestParseTernaryQuestionStillLit(t *testing.T) {
	n, err := ParsePattern(`(seq (unify a any) ">" (unify b any) ":" (unify a any) "?" (unify b any))`)
	if err != nil {
		t.Fatal(err)
	}
	var sawOpt bool
	var walk func(Node)
	walk = func(n Node) {
		if n.MultiOptional {
			sawOpt = true
		}
		if n.Callee != nil {
			walk(*n.Callee)
		}
		for _, a := range n.Args {
			walk(a)
		}
	}
	walk(n)
	if sawOpt {
		t.Fatal("ternary ? must not become MultiOptional")
	}
}

func TestParseUnifyMultiAllowed(t *testing.T) {
	n, err := ParsePattern(`(* (unify c (ref "go:errors::New")))`)
	if err != nil {
		t.Fatal(err)
	}
	if !n.Multi || !n.Unify || n.As != "c" || n.RepMax != -1 {
		t.Fatalf("got %+v", n)
	}
	core, err := NodeToPat(n)
	if err != nil {
		t.Fatal(err)
	}
	rep, ok := core.(Rep)
	if !ok || rep.Min != 0 || rep.Max >= 0 {
		t.Fatalf("want unbounded star, got %#v", core)
	}
	if _, ok := rep.Body.(Unify); !ok {
		t.Fatalf("want Unify body, got %#v", rep.Body)
	}
}

func TestOptionalSkipAndBind(t *testing.T) {
	dir := t.TempDir()

	srcSkip := []byte("package p\nvar _ = x.z\n")
	path := lewpath.New(dir, "skip.go").String()
	if err := os.WriteFile(path, srcSkip, 0o644); err != nil {
		t.Fatal(err)
	}
	pat, err := ParsePattern(`(seq (token "x") (? (capture m (regex "^y$"))) "." (token "z"))`)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "skip.go", srcSkip, pat)
	if len(ms) < 1 {
		t.Fatalf("want match skipping optional y, got %d", len(ms))
	}
	if _, ok := ms[0].CaptureFirst("m"); ok {
		t.Fatalf("m should be unbound when optional skipped, caps=%v", ms[0].Captures)
	}

	srcBind := []byte("package p\nvar _ = x.y.z\n")
	path2 := lewpath.New(dir, "bind.go").String()
	if err := os.WriteFile(path2, srcBind, 0o644); err != nil {
		t.Fatal(err)
	}
	pat2, err := ParsePattern(`(seq (token "x") "." (? (capture m (regex "^y$"))) "." (token "z"))`)
	if err != nil {
		t.Fatal(err)
	}
	ms2 := mustMatchFile(t, dir, path2, "bind.go", srcBind, pat2)
	if len(ms2) < 1 {
		t.Fatalf("want match with y, got 0")
	}
	sp, ok := ms2[0].CaptureFirst("m")
	if !ok || string(sp.Bytes(srcBind)) != "y" {
		t.Fatalf("want m=y, caps=%v", ms2[0].Captures)
	}
}

func TestUnifyMultiEqualAndReject(t *testing.T) {
	dir := t.TempDir()
	// Exact two string sites (word content); gap commas do not match ^[a-z]+$.
	// min=max=2 forces both sites into unify.
	mk := func() Node {
		n, err := PatToNode(Seq{Items: []Pat{
			Regex{Equals: "start"},
			Lit{Text: `,`},
			Rep{Min: 2, Max: 2, Body: Unify{Name: "a", Body: Regex{RE: `^[a-z]+$`}}},
			Lit{Text: `,`},
			Regex{Equals: "end"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	pat := mk()

	srcSame := []byte("package p\nvar _ = []string{\"start\", \"foo\", \"foo\", \"end\"}\n")
	srcDiff := []byte("package p\nvar _ = []string{\"start\", \"foo\", \"bar\", \"end\"}\n")

	path := lewpath.New(dir, "same.go").String()
	if err := os.WriteFile(path, srcSame, 0o644); err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "same.go", srcSame, pat)
	if len(ms) < 1 {
		t.Fatalf("same strings: want match, got %d", len(ms))
	}
	sp, ok := ms[0].CaptureFirst("a")
	if !ok || string(sp.Bytes(srcSame)) != `"foo"` {
		t.Fatalf("want a=\"foo\", got caps=%v", ms[0].Captures)
	}

	path2 := lewpath.New(dir, "diff.go").String()
	if err := os.WriteFile(path2, srcDiff, 0o644); err != nil {
		t.Fatal(err)
	}
	ms2 := mustMatchFile(t, dir, path2, "diff.go", srcDiff, pat)
	if len(ms2) != 0 {
		t.Fatalf("diff strings: want 0 (unify fail), got %d", len(ms2))
	}
}

func TestNumericRepMinMax(t *testing.T) {
	dir := t.TempDir()

	// Body only matches single-letter string contents a|b|c.
	mk := func(min, max int) Node {
		n, err := PatToNode(Seq{Items: []Pat{
			Regex{Equals: "start"},
			Lit{Text: `,`},
			Rep{Min: min, Max: max, Body: Capture{Name: "a", Body: Regex{RE: `^[abc]$`}}},
			Lit{Text: `,`},
			Regex{Equals: "end"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	run := func(src []byte, name string, min, max int) int {
		path := lewpath.New(dir, name+".go").String()
		if err := os.WriteFile(path, src, 0o644); err != nil {
			t.Fatal(err)
		}
		return len(mustMatchFile(t, dir, path, name+".go", src, mk(min, max)))
	}

	// two letter sites
	src2 := []byte("package p\nvar _ = []string{\"start\", \"a\", \"b\", \"end\"}\n")
	// one letter site
	src1 := []byte("package p\nvar _ = []string{\"start\", \"a\", \"end\"}\n")
	// three letter sites
	src3 := []byte("package p\nvar _ = []string{\"start\", \"a\", \"b\", \"c\", \"end\"}\n")

	if c := run(src2, "min2ok", 2, 2); c < 1 {
		t.Fatalf("exact 2 available: want match, got %d", c)
	}
	if c := run(src1, "min2fail", 2, 2); c != 0 {
		t.Fatalf("only 1 site for min=2: want 0, got %d", c)
	}
	// Gap multi may pick any 2 of 3 — max=2 still matches (document product semantics).
	if c := run(src3, "max2pick", 2, 2); c < 1 {
		t.Fatalf("3 sites max=2: gap multi still matches by picking 2, got %d", c)
	}
	if c := run(src3, "min3", 3, 3); c < 1 {
		t.Fatalf("exact 3: want match, got %d", c)
	}
	if c := run(src2, "min3fail", 3, 3); c != 0 {
		t.Fatalf("only 2 sites for min=3: want 0, got %d", c)
	}
	if c := run(src1, "range23", 2, 3); c != 0 {
		t.Fatalf("1 site for 2..3: want 0, got %d", c)
	}
	if c := run(src2, "range23ok", 2, 3); c < 1 {
		t.Fatalf("2 sites for 2..3: want match, got %d", c)
	}
}

func TestCompileRoundTripPhase2(t *testing.T) {
	for _, pat := range []string{
		`(? (capture c (ref "go:fmt::Errorf")))`,
		`(* (unify c (ref "go:errors::New")))`,
		`(+ (unify c (ref "go:errors::New")))`,
		`(seq (unify a any) ">" (unify b any) ":" (unify a any) "?" (unify b any))`,
		`(seq (capture F (ref "go:fmt::Errorf")) "(" (* any) (capture E (ref "go:errors::New")) "(" (* any) ")" (* any) ")")`,
		`(seq "start" "," (+ (unify a (regex "^[a-z]+$"))) "," "end")`,
	} {
		t.Run(pat, func(t *testing.T) {
			n, err := ParsePattern(pat)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := compilePattern(n); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCheckPatAllowsUnifyMulti(t *testing.T) {
	p := Rep{Min: 0, Max: -1, Body: Unify{Name: "a", Body: Any{}}}
	if err := CheckPat(p); err != nil {
		t.Fatal(err)
	}
}

func TestBareOptionalTight(t *testing.T) {
	core, err := ParseToPat(`(? (unify a any))`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := core.(Alt); !ok {
		t.Fatalf("want expanded optional Alt, got %#v", core)
	}
}

func TestInvertGroupRejectsOptional(t *testing.T) {
	// Lookbehind + local optional is a CLI-only parse reject. Sexp allows composing
	// (? (assert_not_behind …)); CheckPat does not ban it.
	_, err := ParsePattern(`(? (assert_not_behind (token "a")))`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseRefQuantNotInRef(t *testing.T) {
	core, err := ParseToPat(`(+ (capture c (ref "go:fmt::Errorf")))`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := core.(Seq); !ok {
		if _, ok := core.(Rep); !ok {
			t.Fatalf("want + expand to Seq or Rep, got %#v", core)
		}
	}
}
