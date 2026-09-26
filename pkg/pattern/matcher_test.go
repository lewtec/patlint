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

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func testSess() *project.Session {
	return project.NewSession(".").WithEngine(ccgo.Engine{})
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
	if err != nil {
		t.Fatal(err)
	}
	pm, ok := m.(PathM)
	if !ok || pm.Glob != "**/*_test.go" {
		t.Fatalf("%#v", m)
	}
	if _, ok := pm.Body.(LeafM); !ok {
		t.Fatalf("body %#v", pm.Body)
	}

	m2, err := ParseMatcher(`(under (token "func") (token "x"))`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m2.(UnderM); !ok {
		t.Fatalf("%#v", m2)
	}
}

func TestCompileMatcherRejectsDoubleFinderAnd(t *testing.T) {
	m, err := ParseMatcher(`(and (token "a") (token "b"))`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CompileMatcher(m)
	if err == nil {
		t.Fatal("want error for and of two finders")
	}
}

func TestCompileMatcherRejectsNotFinder(t *testing.T) {
	m, err := ParseMatcher(`(not (token "a"))`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CompileMatcher(m)
	if err == nil {
		t.Fatal("want error for not of finder")
	}
}

func TestMatcherPathGatesFile(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) {}\n")
	mustWrite(t, lewpath.New(dir, "a.go").String(), src)
	mustWrite(t, lewpath.New(dir, "a_test.go").String(), src)

	m, err := ParseMatcher(`(path "**/*_test.go" (token "interface{}"))`)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	if cm.AcceptsFile("a.go") {
		t.Fatal("a.go should be rejected by path")
	}
	if !cm.AcceptsFile("a_test.go") {
		t.Fatal("a_test.go should be accepted")
	}

	ms := mustMatchMatcher(t, dir, "a_test.go", src, cm)
	if len(ms) < 1 {
		t.Fatalf("want hits in test file, got %d", len(ms))
	}
	ms2 := mustMatchMatcher(t, dir, "a.go", src, cm)
	if len(ms2) != 0 {
		t.Fatalf("want no hits when AcceptsFile false path still enforced in MatchFileMatcher, got %d", len(ms2))
	}
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
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchMatcher(t, dir, "x.go", src, cm)
	if len(ms) != 1 {
		t.Fatalf("want 1 hit inside g, got %d", len(ms))
	}

	// Region only f — no interface{}
	m2, err := ParseMatcher(`(under
  (seq (token "func") (token "f") "(" ")" "{" (* any) "}")
  (token "interface{}"))`)
	if err != nil {
		t.Fatal(err)
	}
	cm2, err := CompileMatcher(m2)
	if err != nil {
		t.Fatal(err)
	}
	ms2 := mustMatchMatcher(t, dir, "x.go", src, cm2)
	if len(ms2) != 0 {
		t.Fatalf("want 0 hits inside f, got %d", len(ms2))
	}
}

func TestMatcherAndPathNot(t *testing.T) {
	m, err := ParseMatcher(`(and (path "**/*.go") (not (path "**/skip/**")) (token "interface{}"))`)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	if !cm.AcceptsFile("pkg/a.go") {
		t.Fatal("want accept pkg/a.go")
	}
	if cm.AcceptsFile("pkg/skip/a.go") {
		t.Fatal("want reject skip/")
	}
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
		t.Fatalf("missing func capture: %#v", ms[0].Captures)
	}
	name := string(src[sp.StartByte:sp.EndByte])
	if name != "TestFoo" {
		t.Fatalf("func=%q want TestFoo", name)
	}
	// Hit span should be around t.Context, not the whole function.
	hit := string(src[ms[0].StartByte:ms[0].EndByte])
	if !strings.Contains(hit, "Context") {
		t.Fatalf("hit span %q should be t.Context site", hit)
	}
}

func TestMatcherLeafCLI(t *testing.T) {
	m, err := ParseMatcher(`(token "interface{}")`)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) {}\n")
	mustWrite(t, lewpath.New(dir, "x.go").String(), src)
	ms := mustMatchMatcher(t, dir, "x.go", src, cm)
	if len(ms) < 1 {
		t.Fatal("want leaf match")
	}
}

func TestCompileMatcherPrecompilesLeafNFA(t *testing.T) {
	m, err := ParseMatcher(`(token "interface{}")`)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	leaf, ok := cm.root.(*finderLeaf)
	if !ok {
		t.Fatalf("root type %T want *finderLeaf", cm.root)
	}
	if leaf.nfa == nil || len(leaf.nfa.states) == 0 {
		t.Fatal("leaf NFA not precompiled")
	}
	// Same hits as MatchFilePat (compile-on-match path).
	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) {}\n")
	mustWrite(t, lewpath.New(dir, "x.go").String(), src)
	msPlan := mustMatchMatcher(t, dir, "x.go", src, cm)
	rootNode := mustParseRoot(t, lewpath.New(dir, "x.go").String())
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	msPat, err := matchFilePatPol(testSess(), dir, "x.go", src, rootNode, leaf.pat, nil, vm.TapePolicy("x.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(msPlan) != len(msPat) {
		t.Fatalf("plan=%d pat=%d", len(msPlan), len(msPat))
	}
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
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchMatcher(t, dir, "x.go", src, cm)
	if len(ms) != 1 {
		t.Fatalf("want 1 hit inside function, got %d", len(ms))
	}
	hit := string(src[ms[0].StartByte:ms[0].EndByte])
	if hit != "interface{}" {
		t.Fatalf("hit=%q", hit)
	}

	// Without node, both sites match.
	m2, err := ParseMatcher(`(token "interface{}")`)
	if err != nil {
		t.Fatal(err)
	}
	cm2, err := CompileMatcher(m2)
	if err != nil {
		t.Fatal(err)
	}
	ms2 := mustMatchMatcher(t, dir, "x.go", src, cm2)
	if len(ms2) != 2 {
		t.Fatalf("want 2 full-file hits, got %d", len(ms2))
	}
}

func TestMatcherAsLanguageReparse(t *testing.T) {
	dir := t.TempDir()
	// Host Go; raw string holds JS. Match curl only after reparse as javascript.
	src := []byte("package p\nvar s = `function f() { curl() }`\n")
	mustWrite(t, lewpath.New(dir, "x.go").String(), src)

	m, err := ParseMatcher(`(as-language "javascript"
  (node "raw_string_literal")
  (token "curl"))`)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchMatcher(t, dir, "x.go", src, cm)
	if len(ms) != 1 {
		t.Fatalf("want 1 hit, got %d spans=%v", len(ms), ms)
	}
	hit := string(src[ms[0].StartByte:ms[0].EndByte])
	if hit != "curl" {
		t.Fatalf("hit=%q want curl (host bytes)", hit)
	}
	// Host-only match must not see curl as a Go token.
	m2, err := ParseMatcher(`(token "curl")`)
	if err != nil {
		t.Fatal(err)
	}
	cm2, err := CompileMatcher(m2)
	if err != nil {
		t.Fatal(err)
	}
	ms2 := mustMatchMatcher(t, dir, "x.go", src, cm2)
	if len(ms2) != 0 {
		t.Fatalf("host tape should not match curl, got %d", len(ms2))
	}
}

func TestParseMatcherNodeAndAsLanguage(t *testing.T) {
	m, err := ParseMatcher(`(node "function_declaration")`)
	if err != nil {
		t.Fatal(err)
	}
	nm, ok := m.(NodeM)
	if !ok || nm.Type != "function_declaration" {
		t.Fatalf("%#v", m)
	}

	m2, err := ParseMatcher(`(as-language "javascript" (node "raw_string_literal") (token "x"))`)
	if err != nil {
		t.Fatal(err)
	}
	al, ok := m2.(AsLangM)
	if !ok || al.Lang != "javascript" {
		t.Fatalf("%#v", m2)
	}
	if _, ok := al.Region.(NodeM); !ok {
		t.Fatalf("region %#v", al.Region)
	}
}

func TestNodeBindsFieldCaptures(t *testing.T) {
	m, err := ParseMatcher(`(node "function_declaration")`)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := []byte("package p\n\nfunc Hello(x int) int {\n\treturn x\n}\n")
	mustWrite(t, lewpath.New(dir, "x.go").String(), src)
	ms := mustMatchMatcher(t, dir, "x.go", src, cm)
	if len(ms) != 1 {
		t.Fatalf("want 1 function, got %d", len(ms))
	}
	pub := PublicCaptures(ms[0], src)
	if pub["name"] != "Hello" {
		t.Fatalf("name=%q want Hello; caps=%v", pub["name"], pub)
	}
	if !strings.Contains(pub["parameters"], "x int") {
		t.Fatalf("parameters=%q want param list; caps=%v", pub["parameters"], pub)
	}
	if !strings.Contains(pub["body"], "return") {
		t.Fatalf("body=%q want block; caps=%v", pub["body"], pub)
	}
	if pub["result"] != "int" {
		t.Fatalf("result=%q want int; caps=%v", pub["result"], pub)
	}
}

func TestCompileMatcherFlattensNestedOr(t *testing.T) {
	m, err := ParseMatcher(`(or (or (token "a") (token "b")) (token "c"))`)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	or, ok := cm.root.(*finderOr)
	if !ok {
		t.Fatalf("root %T", cm.root)
	}
	if len(or.arms) != 3 {
		t.Fatalf("arms=%d want 3 (flattened)", len(or.arms))
	}
	for i, a := range or.arms {
		if _, ok := a.(*finderLeaf); !ok {
			t.Fatalf("arm %d type %T want *finderLeaf", i, a)
		}
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMatchMatcher(t *testing.T, dir, rel string, src []byte, cm *CompiledMatcher) []Match {
	t.Helper()
	abs := lewpath.New(dir, rel).String()
	// parse via MatchFile path helpers
	rootNode := mustParseRoot(t, abs)
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	ms, err := matchFileMatcherPol(testSess(), dir, rel, src, rootNode, cm, nil, vm.TapePolicy(rel), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func TestFileScratchSameMatchesAsFreshTape(t *testing.T) {
	src := []byte("package p\n\nimport \"fmt\"\n\nfunc Hello(x int) {\n\tfmt.Println(x)\n}\n")
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, src, "x.go", "go")
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	pol := vm.TapePolicy("x.go")
	cms := make([]*CompiledMatcher, 0, len(pats))
	want := map[string]struct{}{}
	for _, p := range pats {
		m, err := ParseMatcher(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		cm, err := CompileMatcher(m)
		if err != nil {
			t.Fatalf("compile %s: %v", p, err)
		}
		addFinderNodeTypes(cm.root, want)
		cms = append(cms, cm)
	}
	scratch := &fileScratch{pol: pol, forLang: vm.tapePolicyForLang, want: want}
	for i, cm := range cms {
		p := pats[i]
		fresh, err := matchFileMatcherPol(sess, ".", "x.go", src, pf.Root, cm, nil, pol, nil)
		if err != nil {
			t.Fatal(err)
		}
		shared, err := matchFileMatcherPol(sess, ".", "x.go", src, pf.Root, cm, nil, pol, scratch)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := matchKey(shared), matchKey(fresh); got != want {
			t.Fatalf("%s: scratch matches differ from fresh tape\nfresh=%s\nshared=%s", p, want, got)
		}
	}
	if !scratch.built {
		t.Fatal("scratch tape was not built")
	}
}

func TestNodeIndexUnderSameAsWalk(t *testing.T) {
	src := []byte("package p\n\nfunc A() { x := 1 }\nfunc B() { y := 2; z := 3 }\n")
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, src, "x.go", "go")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pf.Close)
	m, err := ParseMatcher(`(under (node "function_declaration") (node "identifier"))`)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	sess := testSess()
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	pol := vm.TapePolicy("x.go")
	fresh, err := matchFileMatcherPol(sess, ".", "x.go", src, pf.Root, cm, nil, pol, nil)
	if err != nil {
		t.Fatal(err)
	}
	scratch := &fileScratch{pol: pol, want: nodeTypesFromFinder(cm.root)}
	indexed, err := matchFileMatcherPol(sess, ".", "x.go", src, pf.Root, cm, nil, pol, scratch)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := matchKey(indexed), matchKey(fresh); got != want {
		t.Fatalf("under+node index differs from walk\nfresh=%s\nindex=%s", want, got)
	}
	if scratch.index == nil {
		t.Fatal("expected node index")
	}
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
