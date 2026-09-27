package pattern

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func testSess() *project.Session {
	return project.NewSession(".").WithEngine(treesitter.Engine{})
}

func TestMatchPathGlob(t *testing.T) {
	cases := []struct {
		pat, rel string
		want     bool
	}{
		{"**/*_test.go", "pkg/foo_test.go", true},
		{"**/*_test.go", "pkg/foo.go", false},
		{"*.go", "pkg/foo.go", true},
		{"pkg/**", "pkg/a/b.go", true},
		{"pkg/**", "other/a.go", false},
		{"**/testdata/**", "pkg/testdata/x.go", true},
	}
	for _, tc := range cases {
		if got := matchPathGlob(tc.pat, tc.rel); got != tc.want {
			t.Errorf("matchPathGlob(%q,%q)=%v want %v", tc.pat, tc.rel, got, tc.want)
		}
	}
}

func TestParseMatcherPathUnder(t *testing.T) {
	m, err := ParseMatcher(`(path "**/*_test.go" (token "interface{}"))`)
	require.NoError(t, err)

	pm, ok := m.(PathM)
	require.False(t, !ok || pm.Glob != "**/*_test.go",
		"%#v", m)
	{

		_, ok := pm.Body.(LeafM)
		require.True(t, ok,
			"body %#v", pm.Body)
	}

	m2, err := ParseMatcher(`(under (token "func") (token "x"))`)
	require.NoError(t, err)
	{

		_, ok := m2.(UnderM)
		require.True(t, ok,
			"%#v", m2)
	}

}

func TestCompileMatcherRejectsDoubleFinderAnd(t *testing.T) {
	m, err := ParseMatcher(`(and (token "a") (token "b"))`)
	require.NoError(t, err)

	_, err = CompileMatcher(m)
	require.Error(t, err,
		"want error for and of two finders")

}

func TestCompileMatcherRejectsNotFinder(t *testing.T) {
	m, err := ParseMatcher(`(not (token "a"))`)
	require.NoError(t, err)

	_, err = CompileMatcher(m)
	require.Error(t, err,
		"want error for not of finder")

}

func TestMatcherPathGatesFile(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) {}\n")
	mustWrite(t, lewpath.New(dir, "a.go").String(), src)
	mustWrite(t, lewpath.New(dir, "a_test.go").String(), src)

	m, err := ParseMatcher(`(path "**/*_test.go" (token "interface{}"))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)
	require.False(t, cm.AcceptsFile("a.go"),
		"a.go should be rejected by path")
	require.True(t, cm.AcceptsFile("a_test.go"),
		"a_test.go should be accepted")

	ms := mustMatchMatcher(t, dir, "a_test.go", src, cm)
	require.GreaterOrEqual(t, len(ms), 1,
		"want hits in test file, got %d", len(ms))

	ms2 := mustMatchMatcher(t, dir, "a.go", src, cm)
	require.Empty(t, ms2,
		"want no hits when AcceptsFile false path still enforced in MatchFileMatcher, got %d", len(ms2))

}

func TestMatcherUnderRestrictsDomain(t *testing.T) {
	dir := t.TempDir()
	// interface{} only inside second function body should still match with under func…
	// Simpler: under a seq that matches "func g" block containing interface{}
	src := []byte(`package p
func f() { var _ int }
func g() { var _ interface{} }
`)
	path := lewpath.New(dir, "x.go").String()
	mustWrite(t, path, src)

	// Region: func g ( ... ) { ... }  — use gap rest
	// (under (seq (token "func") (token "g") …) (token "interface{}"))
	m, err := ParseMatcher(`(under
  (seq (token "func") (token "g") "(" ")" "{" (* any) "}")
  (token "interface{}"))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	ms := mustMatchMatcher(t, dir, "x.go", src, cm)
	require.Len(t, ms, 1,
		"want 1 hit inside g, got %d", len(ms))

	// Region only f — no interface{}
	m2, err := ParseMatcher(`(under
  (seq (token "func") (token "f") "(" ")" "{" (* any) "}")
  (token "interface{}"))`)
	require.NoError(t, err)

	cm2, err := CompileMatcher(m2)
	require.NoError(t, err)

	ms2 := mustMatchMatcher(t, dir, "x.go", src, cm2)
	require.Empty(t, ms2,
		"want 0 hits inside f, got %d", len(ms2))

}

func TestMatcherAndPathNot(t *testing.T) {
	m, err := ParseMatcher(`(and (path "**/*.go") (not (path "**/skip/**")) (token "interface{}"))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)
	require.True(t, cm.AcceptsFile("pkg/a.go"),
		"want accept pkg/a.go")
	require.False(t, cm.AcceptsFile("pkg/skip/a.go"),
		"want reject skip/")

}

func TestMatcherUnderMergesRegionCaptures(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p
func TestFoo(t *testing.T) { _ = t.Context() }
func TestBar(t *testing.T) { }
`)
	mustWrite(t, lewpath.New(dir, "x_test.go").String(), src)

	m, err := ParseMatcher(`(path "**/*_test.go"
  (under
    (seq (token "func") (capture func (regex "^Test")) "(" (* any) ")" "{" (* any) "}")
    (seq (token "t") "." (token "Context"))))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	ms := mustMatchMatcher(t, dir, "x_test.go", src, cm)
	require.Len(t, ms, 1,
		"want 1 hit, got %d", len(ms))

	sp, ok := ms[0].CaptureFirst("func")
	require.True(t, ok,
		"missing func capture: %#v", ms[0].Captures)

	name := string(src[sp.StartByte:sp.EndByte])
	require.Equal(t, "TestFoo", name,
		"func=%q want TestFoo", name)

	// Hit span should be around t.Context, not the whole function.
	hit := string(src[ms[0].StartByte:ms[0].EndByte])
	require.True(t, strings.Contains(hit, "Context"),
		"hit span %q should be t.Context site", hit)

}

func TestMatcherLeafCLI(t *testing.T) {
	m, err := ParseMatcher(`(token "interface{}")`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) {}\n")
	mustWrite(t, lewpath.New(dir, "x.go").String(), src)
	ms := mustMatchMatcher(t, dir, "x.go", src, cm)
	require.GreaterOrEqual(t, len(ms), 1,
		"want leaf match")

}

func TestCompileMatcherPrecompilesLeafNFA(t *testing.T) {
	m, err := ParseMatcher(`(token "interface{}")`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	leaf, ok := cm.root.(*finderLeaf)
	require.True(t, ok,
		"root type %T want *finderLeaf", cm.root)
	require.False(t, leaf.nfa == nil || len(leaf.nfa.states) == 0,
		"leaf NFA not precompiled")

	// Same hits as MatchFilePat (compile-on-match path).
	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) {}\n")
	mustWrite(t, lewpath.New(dir, "x.go").String(), src)
	msPlan := mustMatchMatcher(t, dir, "x.go", src, cm)
	rootNode := mustParseRoot(t, lewpath.New(dir, "x.go").String())
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	msPat, err := matchFilePatPol(testSess(), dir, "x.go", src, rootNode, leaf.pat, nil, vm.TapePolicy("x.go"))
	require.NoError(t, err)
	require.Len(t, msPlan, len(msPat),
		"plan=%d pat=%d", len(msPlan), len(msPat))

}

func TestMatcherNodeRestrictsDomain(t *testing.T) {
	dir := t.TempDir()
	// interface{} only inside a function_declaration — not at package level.
	src := []byte(`package p
var _ interface{}
func g() { var _ interface{} }
`)
	mustWrite(t, lewpath.New(dir, "x.go").String(), src)

	m, err := ParseMatcher(`(under (node "function_declaration") (token "interface{}"))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	ms := mustMatchMatcher(t, dir, "x.go", src, cm)
	require.Len(t, ms, 1,
		"want 1 hit inside function, got %d", len(ms))

	hit := string(src[ms[0].StartByte:ms[0].EndByte])
	require.Equal(t, "interface{}", hit,
		"hit=%q", hit)

	// Without node, both sites match.
	m2, err := ParseMatcher(`(token "interface{}")`)
	require.NoError(t, err)

	cm2, err := CompileMatcher(m2)
	require.NoError(t, err)

	ms2 := mustMatchMatcher(t, dir, "x.go", src, cm2)
	require.Len(t, ms2, 2,
		"want 2 full-file hits, got %d", len(ms2))

}

func TestMatcherAsLanguageReparse(t *testing.T) {
	dir := t.TempDir()
	// Host Go; raw string holds JS. Match curl only after reparse as javascript.
	src := []byte("package p\nvar s = `function f() { curl() }`\n")
	mustWrite(t, lewpath.New(dir, "x.go").String(), src)

	m, err := ParseMatcher(`(as-language "javascript"
  (node "raw_string_literal")
  (token "curl"))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	ms := mustMatchMatcher(t, dir, "x.go", src, cm)
	require.Len(t, ms, 1,
		"want 1 hit, got %d spans=%v", len(ms), ms)

	hit := string(src[ms[0].StartByte:ms[0].EndByte])
	require.Equal(t, "curl", hit,
		"hit=%q want curl (host bytes)", hit)

	// Host-only match must not see curl as a Go token.
	m2, err := ParseMatcher(`(token "curl")`)
	require.NoError(t, err)

	cm2, err := CompileMatcher(m2)
	require.NoError(t, err)

	ms2 := mustMatchMatcher(t, dir, "x.go", src, cm2)
	require.Empty(t, ms2,
		"host tape should not match curl, got %d", len(ms2))

}

func TestParseMatcherNodeAndAsLanguage(t *testing.T) {
	m, err := ParseMatcher(`(node "function_declaration")`)
	require.NoError(t, err)

	nm, ok := m.(NodeM)
	require.False(t, !ok || nm.Type != "function_declaration",
		"%#v", m)

	m2, err := ParseMatcher(`(as-language "javascript" (node "raw_string_literal") (token "x"))`)
	require.NoError(t, err)

	al, ok := m2.(AsLangM)
	require.False(t, !ok || al.Lang != "javascript",
		"%#v", m2)
	{

		_, ok := al.Region.(NodeM)
		require.True(t, ok,
			"region %#v", al.Region)
	}

}

func TestNodeBindsFieldCaptures(t *testing.T) {
	m, err := ParseMatcher(`(node "function_declaration")`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	dir := t.TempDir()
	src := []byte("package p\n\nfunc Hello(x int) int {\n\treturn x\n}\n")
	mustWrite(t, lewpath.New(dir, "x.go").String(), src)
	ms := mustMatchMatcher(t, dir, "x.go", src, cm)
	require.Len(t, ms, 1,
		"want 1 function, got %d", len(ms))

	pub := PublicCaptures(ms[0], src)
	require.Equal(t, "Hello", pub["name"],
		"name=%q want Hello; caps=%v", pub["name"], pub)
	require.True(t, strings.Contains(pub["parameters"], "x int"),
		"parameters=%q want param list; caps=%v", pub["parameters"], pub)
	require.True(t, strings.Contains(pub["body"], "return"),
		"body=%q want block; caps=%v", pub["body"], pub)
	require.Equal(t, "int", pub["result"],
		"result=%q want int; caps=%v", pub["result"], pub)

}

func TestCompileMatcherFlattensNestedOr(t *testing.T) {
	m, err := ParseMatcher(`(or (or (token "a") (token "b")) (token "c"))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	or, ok := cm.root.(*finderOr)
	require.True(t, ok,
		"root %T", cm.root)
	require.Len(t, or.arms, 3,
		"arms=%d want 3 (flattened)", len(or.arms))

	for i, a := range or.arms {
		{
			_, ok := a.(*finderLeaf)
			require.True(t, ok,
				"arm %d type %T want *finderLeaf", i, a)
		}

	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	err := os.WriteFile(path, data, 0o644)
	require.NoError(t, err)

}

func mustMatchMatcher(t *testing.T, dir, rel string, src []byte, cm *CompiledMatcher) []Match {
	t.Helper()
	abs := lewpath.New(dir, rel).String()
	// parse via MatchFile path helpers
	rootNode := mustParseRoot(t, abs)
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	ms, err := matchFileMatcherPol(t.Context(), testSess(), dir, rel, src, rootNode, cm, nil, vm.TapePolicy(rel), nil)
	require.NoError(t, err)

	return ms
}

func TestFileScratchSameMatchesAsFreshTape(t *testing.T) {
	src := []byte("package p\n\nimport \"fmt\"\n\nfunc Hello(x int) {\n\tfmt.Println(x)\n}\n")
	pf, err := ingestutil.ParseSource(t.Context(), treesitter.Engine{}, src, "x.go", "go")
	require.NoError(t, err)

	t.Cleanup(pf.Close)
	pats := []string{
		`(seq "func" (capture name any) "(")`,
		`(node "identifier")`,
		`(take "field" (node "selector_expression"))`,
		`(seq "import" "\"")`,
		`(node "package_identifier")`,
	}
	sess := testSess()
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	pol := vm.TapePolicy("x.go")
	cms := make([]*CompiledMatcher, 0, len(pats))
	want := map[string]struct{}{}
	for _, p := range pats {
		m, err := ParseMatcher(p)
		require.NoError(t, err,
			"%s: %v", p, err)

		cm, err := CompileMatcher(m)
		require.NoError(t, err,
			"compile %s: %v", p, err)

		addFinderNodeTypes(cm.root, want)
		cms = append(cms, cm)
	}
	scratch := &fileScratch{pol: pol, forLang: vm.tapePolicyForLang, want: want}
	for i, cm := range cms {
		p := pats[i]
		fresh, err := matchFileMatcherPol(t.Context(), sess, ".", "x.go", src, pf.Root, cm, nil, pol, nil)
		require.NoError(t, err)

		shared, err := matchFileMatcherPol(t.Context(), sess, ".", "x.go", src, pf.Root, cm, nil, pol, scratch)
		require.NoError(t, err)
		{

			got, want := matchKey(shared), matchKey(fresh)
			require.Equal(t, want, got,
				"%s: scratch matches differ from fresh tape\nfresh=%s\nshared=%s", p, want, got)
		}

	}
	require.True(t, scratch.built,
		"scratch tape was not built")

}

func TestNodeIndexUnderSameAsWalk(t *testing.T) {
	src := []byte("package p\n\nfunc A() { x := 1 }\nfunc B() { y := 2; z := 3 }\n")
	pf, err := ingestutil.ParseSource(t.Context(), treesitter.Engine{}, src, "x.go", "go")
	require.NoError(t, err)

	t.Cleanup(pf.Close)
	m, err := ParseMatcher(`(under (node "function_declaration") (node "identifier"))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	sess := testSess()
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	pol := vm.TapePolicy("x.go")
	fresh, err := matchFileMatcherPol(t.Context(), sess, ".", "x.go", src, pf.Root, cm, nil, pol, nil)
	require.NoError(t, err)

	scratch := &fileScratch{pol: pol, want: nodeTypesFromFinder(cm.root)}
	indexed, err := matchFileMatcherPol(t.Context(), sess, ".", "x.go", src, pf.Root, cm, nil, pol, scratch)
	require.NoError(t, err)

	got, want := matchKey(indexed), matchKey(fresh)
	require.Equal(t, want, got,
		"under+node index differs from walk\nfresh=%s\nindex=%s", want, got)
	require.NotNil(t, scratch.index,
		"expected node index")

}

func matchKey(ms []Match) string {
	var b strings.Builder
	u := func(n uint32) string { return strconv.FormatUint(uint64(n), 10) }
	for _, m := range ms {
		b.WriteString(m.File)
		b.WriteByte('@')
		b.WriteString(u(m.StartByte))
		b.WriteByte(':')
		b.WriteString(u(m.EndByte))
		names := make([]string, 0, len(m.Captures))
		for name := range m.Captures {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			ss := m.Captures[name]
			b.WriteByte(' ')
			b.WriteString(name)
			for _, s := range ss {
				b.WriteByte(',')
				b.WriteString(u(s.StartByte))
				b.WriteByte('-')
				b.WriteString(u(s.EndByte))
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
