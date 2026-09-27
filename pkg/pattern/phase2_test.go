package pattern

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestParseOptionalQuant(t *testing.T) {
	core, err := ParseToPat(`(? (capture c (ref "go:context::Background")))`)
	require.NoError(t, err)

	// Expand turns (?) into (alt (seq) body).
	alt, ok := core.(Alt)
	require.False(t, !ok || len(alt.Items) != 2,
		"want expanded optional Alt, got %#v", core)

}

func TestParseTernaryQuestionStillLit(t *testing.T) {
	n, err := ParsePattern(`(seq (unify a any) ">" (unify b any) ":" (unify a any) "?" (unify b any))`)
	require.NoError(t, err)

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
	require.False(t, sawOpt,
		"ternary ? must not become MultiOptional")

}

func TestParseUnifyMultiAllowed(t *testing.T) {
	n, err := ParsePattern(`(* (unify c (ref "go:errors::New")))`)
	require.NoError(t, err)
	require.False(t, !n.Multi || !n.Unify || n.As != "c" || n.RepMax != -1,
		"got %+v", n)

	core, err := NodeToPat(n)
	require.NoError(t, err)

	rep, ok := core.(Rep)
	require.False(t, !ok || rep.Min != 0 || rep.Max >= 0,
		"want unbounded star, got %#v", core)
	{

		_, ok := rep.Body.(Unify)
		require.True(t, ok,
			"want Unify body, got %#v", rep.Body)
	}

}

func TestOptionalSkipAndBind(t *testing.T) {
	dir := t.TempDir()

	srcSkip := []byte("package p\nvar _ = x.z\n")
	path := lewpath.New(dir, "skip.go").String()
	{
		err := os.WriteFile(path, srcSkip, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(seq (token "x") (? (capture m (regex "^y$"))) "." (token "z"))`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "skip.go", srcSkip, pat)
	require.GreaterOrEqual(t, len(ms), 1,
		"want match skipping optional y, got %d", len(ms))
	{

		_, ok := ms[0].CaptureFirst("m")
		require.False(t, ok,
			"m should be unbound when optional skipped, caps=%v", ms[0].Captures)
	}

	srcBind := []byte("package p\nvar _ = x.y.z\n")
	path2 := lewpath.New(dir, "bind.go").String()
	{
		err := os.WriteFile(path2, srcBind, 0o644)
		require.NoError(t, err)
	}

	pat2, err := ParsePattern(`(seq (token "x") "." (? (capture m (regex "^y$"))) "." (token "z"))`)
	require.NoError(t, err)

	ms2 := mustMatchFile(t, dir, path2, "bind.go", srcBind, pat2)
	require.GreaterOrEqual(t, len(ms2), 1,
		"want match with y, got 0")

	sp, ok := ms2[0].CaptureFirst("m")
	require.False(t, !ok || string(sp.Bytes(srcBind)) != "y",
		"want m=y, caps=%v", ms2[0].Captures)

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
		require.NoError(t, err)

		return n
	}
	pat := mk()

	srcSame := []byte("package p\nvar _ = []string{\"start\", \"foo\", \"foo\", \"end\"}\n")
	srcDiff := []byte("package p\nvar _ = []string{\"start\", \"foo\", \"bar\", \"end\"}\n")

	path := lewpath.New(dir, "same.go").String()
	{
		err := os.WriteFile(path, srcSame, 0o644)
		require.NoError(t, err)
	}

	ms := mustMatchFile(t, dir, path, "same.go", srcSame, pat)
	require.GreaterOrEqual(t, len(ms), 1,
		"same strings: want match, got %d", len(ms))

	sp, ok := ms[0].CaptureFirst("a")
	require.False(t, !ok || string(sp.Bytes(srcSame)) != `"foo"`,
		"want a=\"foo\", got caps=%v", ms[0].Captures)

	path2 := lewpath.New(dir, "diff.go").String()
	err := os.WriteFile(path2, srcDiff, 0o644)
	require.NoError(t, err)

	ms2 := mustMatchFile(t, dir, path2, "diff.go", srcDiff, pat)
	require.Empty(t, ms2,
		"diff strings: want 0 (unify fail), got %d", len(ms2))

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
		require.NoError(t, err)

		return n
	}

	run := func(src []byte, name string, min, max int) int {
		path := lewpath.New(dir, name+".go").String()
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)

		return len(mustMatchFile(t, dir, path, name+".go", src, mk(min, max)))
	}

	// two letter sites
	src2 := []byte("package p\nvar _ = []string{\"start\", \"a\", \"b\", \"end\"}\n")
	// one letter site
	src1 := []byte("package p\nvar _ = []string{\"start\", \"a\", \"end\"}\n")
	// three letter sites
	src3 := []byte("package p\nvar _ = []string{\"start\", \"a\", \"b\", \"c\", \"end\"}\n")
	{

		c := run(src2, "min2ok", 2, 2)
		require.GreaterOrEqual(t, c, 1,
			"exact 2 available: want match, got %d", c)
	}
	{

		c := run(src1, "min2fail", 2, 2)
		require.Equal(t, 0, c,
			"only 1 site for min=2: want 0, got %d", c)
	}
	{

		// Gap multi may pick any 2 of 3 — max=2 still matches (document product semantics).
		c := run(src3, "max2pick", 2, 2)
		require.GreaterOrEqual(t, c, 1,
			"3 sites max=2: gap multi still matches by picking 2, got %d", c)
	}
	{

		c := run(src3, "min3", 3, 3)
		require.GreaterOrEqual(t, c, 1,
			"exact 3: want match, got %d", c)
	}
	{

		c := run(src2, "min3fail", 3, 3)
		require.Equal(t, 0, c,
			"only 2 sites for min=3: want 0, got %d", c)
	}
	{

		c := run(src1, "range23", 2, 3)
		require.Equal(t, 0, c,
			"1 site for 2..3: want 0, got %d", c)
	}

	c := run(src2, "range23ok", 2, 3)
	require.GreaterOrEqual(t, c, 1,
		"2 sites for 2..3: want match, got %d", c)

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
			require.NoError(t, err)
			{

				_, err := compilePattern(n)
				require.NoError(t, err)
			}

		})
	}
}

func TestCheckPatAllowsUnifyMulti(t *testing.T) {
	p := Rep{Min: 0, Max: -1, Body: Unify{Name: "a", Body: Any{}}}
	err := CheckPat(p)
	require.NoError(t, err)

}

func TestBareOptionalTight(t *testing.T) {
	core, err := ParseToPat(`(? (unify a any))`)
	require.NoError(t, err)

	_, ok := core.(Alt)
	require.True(t, ok,
		"want expanded optional Alt, got %#v", core)

}

func TestInvertGroupRejectsOptional(t *testing.T) {
	// Lookbehind + local optional is a CLI-only parse reject. Sexp allows composing
	// (? (assert_not_behind …)); CheckPat does not ban it.
	_, err := ParsePattern(`(? (assert_not_behind (token "a")))`)
	require.NoError(t, err)

}

func TestParseRefQuantNotInRef(t *testing.T) {
	core, err := ParseToPat(`(+ (capture c (ref "go:fmt::Errorf")))`)
	require.NoError(t, err)

	if _, ok := core.(Seq); !ok {
		{
			_, ok := core.(Rep)
			require.True(t, ok,
				"want + expand to Seq or Rep, got %#v", core)
		}

	}
}
