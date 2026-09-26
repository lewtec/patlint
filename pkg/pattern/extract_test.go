package pattern_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestCFunctionAtomIsDeclaratorNotSoup(t *testing.T) {
	src := []byte("#include \"h.h\"\n\nint helper(int a) {\n  return a + 1;\n}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "helper.c")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "helper.c")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range fe.Atoms {
		names = append(names, a.Name)
		switch a.Name {
		case "(", ")", "+", "1", ";", "int", "return", "{", "}":
			t.Fatalf("token-soup atom %q; atoms=%v", a.Name, names)
		}
	}
	found := false
	for _, a := range fe.Atoms {
		if a.Name == "helper" {
			found = true
			if string(src[a.StartByte:a.EndByte]) != "helper" {
				t.Fatalf("helper locus %q", src[a.StartByte:a.EndByte])
			}
		}
	}
	if !found {
		t.Fatalf("want helper atom, have %v", names)
	}
}

func TestGoConstVarAndFieldAtoms(t *testing.T) {
	src := []byte("package p\n\nconst A = 1\nconst (\n\tB, C = 2, 3\n)\n\nvar X int\n\ntype T struct {\n\tF int\n}\n\ntype I interface {\n\tM()\n}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "t.go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "t.go")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, a := range fe.Atoms {
		got[a.Name] = true
	}
	for _, w := range []string{"A", "B", "C", "X", "T", "T.F", "I", "I.M"} {
		if !got[w] {
			t.Errorf("missing atom %q; have %v", w, got)
		}
	}
	if got["F"] {
		t.Error("bare field F should be T.F")
	}
}

func TestJSDestructureAtomsAreBindings(t *testing.T) {
	src := []byte("const [thing, setThing] = useState();\nconst { a, b: c } = obj;\nconst x = 1;\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "a.js")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "a.js")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, a := range fe.Atoms {
		got[a.Name] = true
		if strings.ContainsAny(a.Name, "[]{}") {
			t.Fatalf("pattern soup atom %q; atoms=%v", a.Name, got)
		}
	}
	for _, w := range []string{"thing", "setThing", "a", "c", "x"} {
		if !got[w] {
			t.Errorf("missing atom %q; have %v", w, got)
		}
	}
	if got["b"] {
		t.Error("object key b should not be a binding atom")
	}
	if got["useState"] || got["obj"] {
		t.Errorf("rhs should not be atoms: %v", got)
	}
}

func TestTSDestructureAtomsAreBindings(t *testing.T) {
	src := []byte("const [thing, setThing] = useState<string>();\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "a.ts")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "a.ts")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, a := range fe.Atoms {
		got[a.Name] = true
		if strings.ContainsAny(a.Name, "[]{}") {
			t.Fatalf("pattern soup atom %q", a.Name)
		}
	}
	if !got["thing"] || !got["setThing"] {
		t.Fatalf("have %v", got)
	}
}

func TestPythonAssignmentAtoms(t *testing.T) {
	src := []byte("_TEXT_OPENFLAGS = 1\nX = 2\n\nclass C:\n    Y = 3\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "a.py")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "a.py")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, a := range fe.Atoms {
		got[a.Name] = true
	}
	for _, w := range []string{"_TEXT_OPENFLAGS", "X", "C", "C.Y"} {
		if !got[w] {
			t.Errorf("missing atom %q; have %v", w, got)
		}
	}
}

func TestJSDefaultImportLocalName(t *testing.T) {
	src := []byte("import SearchBar from './SearchBar.js';\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "main.js")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "main.js")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, im := range fe.Imports {
		if im.LocalName == "SearchBar" && im.SourcePath == "./SearchBar.js" {
			found = true
		}
	}
	if !found {
		t.Fatalf("imports=%#v", fe.Imports)
	}
}

func TestJSConstLetAtoms(t *testing.T) {
	src := []byte("export const variant = \"primary\"\nlet query = 1\nfunction greet() {}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "a.js")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "a.js")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, a := range fe.Atoms {
		got[a.Name] = true
	}
	for _, w := range []string{"variant", "query", "greet"} {
		if !got[w] {
			t.Errorf("missing atom %q; have %v", w, got)
		}
	}
}

func TestProductExtractRunsJSPackOnVueScript(t *testing.T) {
	src := []byte("<script>\nexport let initialQuery = ''\nfunction handleSearch() {}\n</script>\n<template><div/></template>\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "App.vue")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "App.vue")
	if err != nil {
		t.Fatal(err)
	}
	if fe.Language != "vue" {
		t.Fatalf("host language=%q", fe.Language)
	}
	got := map[string]bool{}
	for _, a := range fe.Atoms {
		got[a.Name] = true
		if a.Name == "initialQuery" || a.Name == "handleSearch" {
			if string(src[a.StartByte:a.EndByte]) != a.Name {
				t.Errorf("atom %q locus %q", a.Name, src[a.StartByte:a.EndByte])
			}
		}
	}
	for _, w := range []string{"initialQuery", "handleSearch"} {
		if !got[w] {
			t.Fatalf("missing guest atom %q; have %v", w, got)
		}
	}
}

func TestAtomJoinNames_GoMethod(t *testing.T) {
	src := []byte("package p\n\ntype T struct{}\n\nfunc (t *T) Method() {}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "t.go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "t.go")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range fe.Atoms {
		names = append(names, a.Name)
	}
	found := false
	for _, n := range names {
		if n == "T.Method" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want T.Method in atoms %v", names)
	}
}

func TestAtomJoinNames_JavaMethod(t *testing.T) {
	src := []byte("class A {\n  void run() {}\n}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "A.java")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "A.java")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range fe.Atoms {
		names = append(names, a.Name)
	}
	found := false
	for _, n := range names {
		if n == "A.run" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want A.run in atoms %v", names)
	}
	// leaf span is "run", not "A"
	for _, a := range fe.Atoms {
		if a.Name == "A.run" {
			text := string(src[a.StartByte:a.EndByte])
			if text != "run" {
				t.Fatalf("locus text=%q want run", text)
			}
		}
	}
}

func TestUseNameCaptures_JavaObjectName(t *testing.T) {
	src := []byte(`class A {
  int run() { return 1; }
  int x;
  int use(A as) {
    return as.run() + as.x;
  }
}
`)
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(t.Context(), src, "A.java")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "A.java")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"as.run": false, "as.x": false}
	for _, u := range fe.Usages {
		if len(u.Prefix) == 0 {
			continue
		}
		var b strings.Builder
		for _, p := range u.Prefix {
			b.WriteString(p.Name)
			b.WriteByte('.')
		}
		b.WriteString(u.Name)
		key := b.String()
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Fatalf("missing use path %s", k)
		}
	}
}

func TestAsFamilyClaim(t *testing.T) {
	src := `
(under (path "**/*.js")
  (as-language "javascript")
  (as-family "ecma")
  (under (node "identifier")
    (as-use)))
`
	prog, err := pattern.LoadExtractPack("fam.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Families) != 1 || prog.Families[0].Lang != "javascript" || prog.Families[0].Family != "ecma" {
		t.Fatalf("families=%#v", prog.Families)
	}
}

func TestAsFamilyKnobs(t *testing.T) {
	prog, err := pattern.LoadExtractPack("go.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-family "go" directory-module package-scoped-bare-names nested-type-members))
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Families) != 1 {
		t.Fatalf("families=%#v", prog.Families)
	}
	r := prog.Families[0].Rules
	if !r.DirectoryModule || !r.PackageScopedBareNames || !r.NestedTypeMembers {
		t.Fatalf("rules=%#v", r)
	}
	if r.EmptyPackageDirScoped || r.IncludeFileExportsBare || r.DirectoryManifest {
		t.Fatalf("extra knobs=%#v", r)
	}
}

func TestAsFamilyDirectoryManifest(t *testing.T) {
	prog, err := pattern.LoadExtractPack("js.rft", `
(under (path "**/*.js")
  (as-language "javascript")
  (as-family "ecma" directory-manifest))
`)
	if err != nil {
		t.Fatal(err)
	}
	if !prog.Families[0].Rules.DirectoryManifest {
		t.Fatalf("rules=%#v", prog.Families[0].Rules)
	}
}

func TestAsLayoutClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("go.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-layout package import body))
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Layouts) != 1 || prog.Layouts[0].Lang != "go" {
		t.Fatalf("layouts=%#v", prog.Layouts)
	}
	got := strings.Join(prog.Layouts[0].Names, " ")
	if got != "package import body" {
		t.Fatalf("names=%q", got)
	}
}

func TestAsLayoutKeepsPackNames(t *testing.T) {
	prog, err := pattern.LoadExtractPack("go.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-layout package import))
`)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(prog.Layouts[0].Names, " ")
	if got != "package import" {
		t.Fatalf("names=%q", got)
	}
}

func TestAsLayoutDuplicateName(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-layout package package body))
`)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("want duplicate, got %v", err)
	}
}

func TestAsFamilyUnknownKnob(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.js")
  (as-language "javascript")
  (as-family "ecma" no-such-knob))
`)
	if err == nil || !strings.Contains(err.Error(), "no-such-knob") {
		t.Fatalf("want unknown knob, got %v", err)
	}
}

func TestAsFamilyRejectsNameVisibilityKnobs(t *testing.T) {
	for _, knob := range []string{"capital-export", "hide-underscore", "hide-unexported"} {
		_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-family "go" `+knob+`))
`)
		if err == nil || !strings.Contains(err.Error(), knob) {
			t.Fatalf("knob %s: want unknown, got %v", knob, err)
		}
	}
}

func TestAsDirectoryRepresentant(t *testing.T) {
	prog, err := pattern.LoadExtractPack("py.rft", `
(under (path "**/__init__.py")
  (as-directory-representant))
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.DirectoryRepresentants) != 1 || prog.DirectoryRepresentants[0] != "**/__init__.py" {
		t.Fatalf("representants=%#v", prog.DirectoryRepresentants)
	}
}

func TestAsDirectoryRepresentantWantsPath(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `(as-directory-representant)`)
	if err == nil || !strings.Contains(err.Error(), "under (path") {
		t.Fatalf("want path place, got %v", err)
	}
}

func TestAsFamilyUnknownIsClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.js")
  (as-language "javascript")
  (as-family "no_such_family"))
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Families) != 1 || prog.Families[0].Family != "no_such_family" {
		t.Fatalf("families=%#v", prog.Families)
	}
}

func TestAsPaintClassifyLeaf(t *testing.T) {
	prog, err := pattern.LoadExtractPack("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (as-keyword (alt (token "func") (token "var")))
  (as-ident (node "identifier")))
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := prog.ClassifyLeaf("dsl", "func"); got != ingest.HLKeyword {
		t.Fatalf("func=%q", got)
	}
	if got := prog.ClassifyLeaf("dsl", "identifier"); got != ingest.HLIdent {
		t.Fatalf("identifier=%q", got)
	}
	if got := prog.ClassifyLeaf("dsl", "("); got != "" {
		t.Fatalf("punct harvest=%q", got)
	}
}

func TestAsDocstringClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (as-docstring comment-before))
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Docstrings) != 1 || prog.Docstrings[0].Lang != "dsl" || prog.Docstrings[0].Kind != "comment-before" {
		t.Fatalf("docstrings=%#v", prog.Docstrings)
	}
}

func TestAsImportSeqClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (as-import (seq "import" "\"" path "\"")))
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.ImportLines) != 1 {
		t.Fatalf("import-lines=%#v", prog.ImportLines)
	}
	c := prog.ImportLines[0]
	if c.Lang != "dsl" || c.HasLeaf || c.HasQual || !c.QuotedPath || !seqHasPath(c.Tokens) {
		t.Fatalf("import-lines=%#v", prog.ImportLines)
	}
}

func seqHasPath(toks []string) bool {
	for _, t := range toks {
		if t == "path" {
			return true
		}
	}
	return false
}

func TestAsPackageSeqClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (as-package (seq "package" pkg)))
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.PackageLines) != 1 {
		t.Fatalf("package-lines=%#v", prog.PackageLines)
	}
	c := prog.PackageLines[0]
	if c.Lang != "dsl" || len(c.Tokens) != 2 || c.Tokens[0] != "package" || c.Tokens[1] != "pkg" {
		t.Fatalf("package-lines=%#v", prog.PackageLines)
	}
}

func TestAsAtomicClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (as-atomic "interface{}"))
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.AtomicSpans) != 1 || prog.AtomicSpans[0].Lang != "dsl" || prog.AtomicSpans[0].Text != "interface{}" {
		t.Fatalf("atomic=%#v", prog.AtomicSpans)
	}
}

func TestAsGrammarClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (under (node "sym_lit")
    (as-ident)))
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Grammars) != 1 || prog.Grammars[0].Lang != "dsl" || prog.Grammars[0].Grammar != "commonlisp" {
		t.Fatalf("grammars=%#v", prog.Grammars)
	}
}

func TestAsGrammarUnknownFails(t *testing.T) {
	prog, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "no_such_grammar"))
`)
	if err != nil {
		t.Fatal(err)
	}
	err = prog.ValidateGrammars(t.Context(), ccgo.Engine{})
	if !errors.Is(err, pattern.ErrExtract) {
		t.Fatalf("err=%v", err)
	}
}

func TestAsGrammarNeedsLanguageFirst(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.dsl")
  (as-grammar "commonlisp")
  (as-language "dsl"))
`)
	if !errors.Is(err, pattern.ErrExtract) {
		t.Fatalf("err=%v", err)
	}
}

func TestAsLanguageWithoutGrammarFails(t *testing.T) {
	prog, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.dsl")
  (as-language "dsl"))
`)
	if err != nil {
		t.Fatal(err)
	}
	err = prog.ValidateGrammars(t.Context(), ccgo.Engine{})
	if !errors.Is(err, pattern.ErrExtract) {
		t.Fatalf("err=%v", err)
	}
}

func TestPurposeLispUsesCommonlispGrammar(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (under (node "sym_lit")
    (as-ident)))
`))
	if err != nil {
		t.Fatal(err)
	}
	lang, ok := vm.HostLanguage("app.dsl")
	if !ok || lang != "dsl" {
		t.Fatalf("host=%q ok=%v", lang, ok)
	}
	if got := vm.GrammarForLanguage("dsl"); got != "commonlisp" {
		t.Fatalf("grammar=%q", got)
	}
	dir := t.TempDir()
	path := lewpath.New(dir, "app.dsl").String()
	if err := os.WriteFile(path, []byte("(hello world)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	var got *project.FileExtract
	err = w.WalkExtracts(t.Context(), ingest.SourceProject(dir), func(fe *project.FileExtract) bool {
		got = fe
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Language != "dsl" {
		t.Fatalf("extract=%#v", got)
	}
}

func TestAsFamilyNeedsLanguageFirst(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.js")
  (as-family "ecma")
  (as-language "javascript"))
`)
	if !errors.Is(err, pattern.ErrExtract) {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadExtractPack_GoFragment(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "package_clause")
    (under (node "package_identifier")
      (as-package)))
  (under (node "function_declaration")
    (as-atom public (take "name" (seq "func" (capture name any) "(")))))
`
	prog, err := pattern.LoadExtractPack("test.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	if prog.Language != "go" {
		t.Fatalf("lang=%q", prog.Language)
	}
	// host claim + package + atom
	if len(prog.Actions) != 3 {
		t.Fatalf("actions=%d", len(prog.Actions))
	}
	claim := prog.Actions[0]
	if claim.Matcher != nil || claim.HostLang != "go" {
		t.Fatalf("action0 want host claim, got matcher=%v host=%q", claim.Matcher != nil, claim.HostLang)
	}
	if len(claim.Paths) != 1 || claim.Paths[0] != "**/*.go" {
		t.Fatalf("claim paths=%v", claim.Paths)
	}
	if prog.Actions[1].HostLang != "go" || prog.Actions[1].Lang != "go" {
		t.Fatalf("action1 host/lang=%q/%q", prog.Actions[1].HostLang, prog.Actions[1].Lang)
	}
	if prog.Actions[1].Embed {
		t.Fatal("host action should not be embed")
	}

	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	code := []byte("package demo\nfunc Hello() {}\n")
	if err := os.WriteFile(path, code, 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := ingestutil.ParseSourceFile(t.Context(), ccgo.Engine{}, path, "go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()

	fe, err := prog.ExtractErr(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), pf.Root, code, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if fe.Package != "demo" {
		t.Fatalf("package=%q", fe.Package)
	}
	if len(fe.Atoms) != 1 || fe.Atoms[0].Name != "Hello" {
		t.Fatalf("atoms=%+v", fe.Atoms)
	}
}

func TestLoadExtractPack_UnknownLanguage(t *testing.T) {
	prog, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.x")
  (as-language "no_such_grammar_xyz"))
`)
	if err != nil {
		t.Fatal(err)
	}
	err = prog.ValidateGrammars(t.Context(), ccgo.Engine{})
	if err == nil {
		t.Fatal("want error for unregistered grammar id")
	}
	if !strings.Contains(err.Error(), "no_such_grammar_xyz") {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadExtractPack_RejectsRootAsLanguage(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `(as-language "go")`)
	if err == nil {
		t.Fatal("want error for root as-language")
	}
	if !strings.Contains(err.Error(), "under (path") {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadExtractPack_AsLanguageBody(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go"
    (under (node "package_identifier")
      (as-package))))
`
	prog, err := pattern.LoadExtractPack("test.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	// host claim + as-package
	if prog.Language != "go" || len(prog.Actions) != 2 {
		t.Fatalf("lang=%q actions=%d", prog.Language, len(prog.Actions))
	}
}

func TestExtractPack_IgnoresUnknownHead(t *testing.T) {
	prog, err := pattern.LoadExtractPack("ok.rft", `(top (on "x"))`)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Actions) != 0 {
		t.Fatalf("unknown head must not become extract actions: %d", len(prog.Actions))
	}
}

func TestExtractPack_RejectsStringMatcher(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-atom public "Hello")))
`
	_, err := pattern.LoadExtractPack("bad.rft", src)
	if err == nil {
		t.Fatal("want error for string matcher")
	}
	if !strings.Contains(err.Error(), "MATCHER") {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadEmbeddedLanguagePacks(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	prog := vm.Packs().Program()
	packs := map[string]*pattern.ExtractProgram{}
	for _, act := range prog.Actions {
		h := act.HostLang
		if h == "" {
			h = act.Lang
		}
		if h == "" || act.Embed {
			continue
		}
		p := packs[h]
		if p == nil {
			p = &pattern.ExtractProgram{Language: h}
			packs[h] = p
		}
		p.Actions = append(p.Actions, act)
	}
	want := []string{
		"go", "python", "javascript", "typescript", "tsx",
		"java", "kotlin", "scala", "rust", "zig",
		"c", "cpp", "nix", "vue", "svelte", "astro", "templ", "html",
	}
	for _, lang := range want {
		p, ok := packs[lang]
		if !ok {
			t.Errorf("missing pack %q", lang)
			continue
		}
		if p.Language != lang {
			t.Errorf("%s: Language=%q", lang, p.Language)
		}
		if len(p.Actions) == 0 {
			t.Errorf("%s: no actions", lang)
		}
		var extractors int
		for i, a := range p.Actions {
			if len(a.Paths) == 0 {
				t.Errorf("%s action %d: empty paths", lang, i)
			}
			if a.Lang != lang {
				t.Errorf("%s action %d: Lang=%q", lang, i, a.Lang)
			}
			// Host claim from as-language has nil Matcher; extract/paint do not.
			if a.Matcher != nil {
				extractors++
			} else if a.HostLang == "" {
				t.Errorf("%s action %d: nil matcher without HostLang", lang, i)
			}
		}
		if extractors == 0 {
			t.Errorf("%s: no extract/paint matchers", lang)
		}
	}
	if len(packs) < len(want) {
		t.Fatalf("packs=%d want at least %d", len(packs), len(want))
	}
}

func TestLoadExtractPack_MultiPathGlob(t *testing.T) {
	src := `
(under (path "**/*.js" "**/*.mjs")
  (as-language "javascript")
  (under (node "identifier")
    (as-use)))
`
	prog, err := pattern.LoadExtractPack("test.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Actions) != 2 {
		t.Fatalf("actions=%d", len(prog.Actions))
	}
	if len(prog.Actions[0].Paths) != 2 {
		t.Fatalf("paths=%v", prog.Actions[0].Paths)
	}
}

func TestLoadExtractPack_AsAtomRequiresMatcher(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-atom))
`)
	if err == nil {
		t.Fatal("want error for bare (as-atom)")
	}
	if !strings.Contains(err.Error(), "as-atom wants public|private and MATCHER") {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadExtractPack_AsAtomRequiresVisibility(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-atom (take "name" (node "function_declaration")))))
`)
	if err == nil {
		t.Fatal("want error for as-atom without public|private")
	}
	if !strings.Contains(err.Error(), "as-atom wants public|private and MATCHER") {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadExtractPack_AsAtomRejectsUnknownVisibility(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-atom exported (take "name" (node "function_declaration")))))
`)
	if err == nil {
		t.Fatal("want error for unknown visibility")
	}
	if !strings.Contains(err.Error(), `as-atom wants public or private, got "exported"`) {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadExtractPack_AsAtomThisPlace(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-atom public (this))
    (under (node "identifier")
      (as-use (scope name)))))
`
	prog, err := pattern.LoadExtractPack("ok.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	var atoms, uses int
	for _, a := range prog.Actions {
		switch a.Kind {
		case pattern.ExtractAtom:
			atoms++
			if a.Matcher == nil {
				t.Fatal("as-atom (this) must compile a matcher")
			}
			if !a.Exported {
				t.Fatal("as-atom public (this) must set Exported")
			}
		case pattern.ExtractUse:
			uses++
			if a.ScopeField != "name" || a.ScopeNode != "function_declaration" {
				t.Fatalf("use scope field=%q node=%q", a.ScopeField, a.ScopeNode)
			}
		}
	}
	if atoms != 1 || uses != 1 {
		t.Fatalf("atoms=%d uses=%d", atoms, uses)
	}
}

func TestLoadExtractPack_AsAtomPrivateExported(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "parameter_declaration")
    (as-atom private (take "name" (node "parameter_declaration")))))
`
	prog, err := pattern.LoadExtractPack("ok.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, a := range prog.Actions {
		if a.Kind != pattern.ExtractAtom {
			continue
		}
		saw = true
		if a.Exported {
			t.Fatal("as-atom private must clear Exported")
		}
	}
	if !saw {
		t.Fatal("want as-atom action")
	}
}

func TestLoadExtractPack_AsScopeRequiresMatcher(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-scope))
`)
	if err == nil {
		t.Fatal("want error for bare (as-scope)")
	}
	if !strings.Contains(err.Error(), "as-scope wants exactly one MATCHER") {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadExtractPack_AsScopeThisPlace(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-scope (this))))
`
	prog, err := pattern.LoadExtractPack("ok.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, a := range prog.Actions {
		if a.Kind == pattern.ExtractScope {
			n++
			if a.Matcher == nil {
				t.Fatal("as-scope (this) must compile a matcher")
			}
			if a.NodeType != "function_declaration" {
				t.Fatalf("NodeType=%q", a.NodeType)
			}
		}
	}
	if n != 1 {
		t.Fatalf("scopes=%d", n)
	}
	got := prog.ScopeNodeTypes("go")
	if len(got) != 1 || got[0] != "function_declaration" {
		t.Fatalf("ScopeNodeTypes=%v", got)
	}
}

func TestLoadExtractPack_AsFlow(t *testing.T) {
	prog, err := pattern.LoadExtractPack("ok.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "if_statement")
    (as-flow structural (take "consequence" (node "if_statement"))))
  (under (node "if_statement")
    (as-flow hybrid (take "alternative" (node "if_statement"))))
  (under (node "for_statement")
    (as-flow structural (take "body" (node "for_statement")))))
`)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, a := range prog.Actions {
		if a.Kind == pattern.ExtractFlow {
			n++
			if a.Matcher == nil {
				t.Fatal("as-flow must compile a matcher")
			}
		}
	}
	if n != 3 {
		t.Fatalf("flows=%d", n)
	}
	if _, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "if_statement")
    (as-flow nope (this))))
`); err == nil {
		t.Fatal("want error for bad as-flow class")
	}
}

func TestExtractErr_AsFlowElseIf(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-scope (this))
    (as-atom public (take "name" (node "function_declaration"))))
  (under (node "if_statement")
    (as-flow structural (take "consequence" (node "if_statement"))))
  (under (node "if_statement")
    (as-flow hybrid (take "alternative" (node "if_statement"))))
  (under (node "for_statement")
    (as-flow structural (take "body" (node "for_statement")))))
`
	prog, err := pattern.LoadExtractPack("flow.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	code := []byte(`package p

func Demo(n int) int {
	if n > 0 {
		if n > 1 {
			return 1
		}
	} else if n < 0 {
		return -1
	} else {
		return 0
	}
	for i := 0; i < n; i++ {
		if i == 0 {
			continue
		}
	}
	return n
}
`)
	if err := os.WriteFile(path, code, 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := ingestutil.ParseSourceFile(t.Context(), ccgo.Engine{}, path, "go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := prog.ExtractErr(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), pf.Root, code, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(fe.Flows) == 0 {
		t.Fatal("no flow rows")
	}
	var unit project.ScopeDef
	for _, s := range fe.Scopes {
		if s.StartByte < s.EndByte && strings.Contains(string(code[s.StartByte:s.EndByte]), "func Demo") {
			unit = s
			break
		}
	}
	if unit.EndByte == 0 {
		t.Fatalf("no Demo scope: %+v", fe.Scopes)
	}
	rep := ingest.ScoreFlow(unit.StartByte, unit.EndByte, fe.Flows)
	// if, nested if, else-if, else, for, if-in-for
	if rep.Score < 6 {
		t.Fatalf("score=%d incs=%+v flows=%+v", rep.Score, rep.Incs, fe.Flows)
	}
	var hybrids, structs int
	for _, fl := range fe.Flows {
		switch fl.Class {
		case project.FlowHybrid:
			hybrids++
		case project.FlowStructural:
			structs++
		}
	}
	if hybrids < 2 {
		t.Fatalf("hybrid=%d want >=2 (else-if + else); flows=%+v", hybrids, fe.Flows)
	}
	if structs < 4 {
		t.Fatalf("structural=%d want >=4; flows=%+v", structs, fe.Flows)
	}
}

func TestLoadExtractPack_AsDecl(t *testing.T) {
	prog, err := pattern.LoadExtractPack("ok.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "type_declaration")
    (as-decl (this))))
`)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, a := range prog.Actions {
		if a.Kind == pattern.ExtractScope && a.HoleOnly {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("decls=%d", n)
	}
	if _, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "type_declaration")
    (as-decl (this) "type $")))
`); err == nil {
		t.Fatal("as-decl template should fail")
	}
}

func TestAttributeHostLanguage(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	lang, ok := vm.HostLanguage("pkg/foo.go")
	if !ok || lang != "go" {
		t.Fatalf("got %q ok=%v", lang, ok)
	}
	_, ok = vm.HostLanguage("nope.xyz")
	if ok {
		t.Fatal("expected no claim for .xyz")
	}
}

func TestExtractEmbedAsLanguage(t *testing.T) {
	// Host go file; raw string reparsed as javascript for identifier uses.
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "package_clause")
    (under (node "package_identifier")
      (as-package)))
  (under (node "raw_string_literal")
    (as-language "javascript")
    (under (node "identifier")
      (as-use))))
`
	prog, err := pattern.LoadExtractPack("embed.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	var embedAct *pattern.ExtractAction
	for i := range prog.Actions {
		if prog.Actions[i].Embed {
			embedAct = &prog.Actions[i]
			break
		}
	}
	if embedAct == nil || embedAct.Lang != "javascript" || embedAct.Region == nil {
		t.Fatalf("embed action missing: %+v", prog.Actions)
	}

	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	code := []byte("package demo\nvar s = `function hello() { curl() }`\n")
	if err := os.WriteFile(path, code, 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := ingestutil.ParseSourceFile(t.Context(), ccgo.Engine{}, path, "go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()

	fe, err := prog.ExtractErr(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), pf.Root, code, "x.go")
	if err != nil {
		// javascript grammar may be unregistered in this test binary
		if strings.Contains(err.Error(), "unknown language") {
			t.Skip(err.Error())
		}
		t.Fatal(err)
	}
	if fe.Package != "demo" {
		t.Fatalf("package=%q", fe.Package)
	}
	// When JS grammar is linked, expect identifier uses inside the raw string.
	_ = fe.Usages
}

func TestExtractErr_AsAtomVisibility(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-atom public (take "name" (node "function_declaration"))))
  (under (node "parameter_declaration")
    (as-atom private (take "name" (node "parameter_declaration")))))
`
	prog, err := pattern.LoadExtractPack("vis.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	code := []byte("package p\n\nfunc Hello(x int) {}\n")
	if err := os.WriteFile(path, code, 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := ingestutil.ParseSourceFile(t.Context(), ccgo.Engine{}, path, "go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := prog.ExtractErr(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), pf.Root, code, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, a := range fe.Atoms {
		got[a.Name] = a.Exported
	}
	if !got["Hello"] {
		t.Fatalf("Hello exported=%v atoms=%v", got["Hello"], fe.Atoms)
	}
	if exp, ok := got["x"]; !ok || exp {
		t.Fatalf("x private want Exported=false, got present=%v exported=%v atoms=%v", ok, exp, fe.Atoms)
	}
}

func TestExtractErr_AsScopeNestByContainment(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "type_spec")
    (as-atom public (take "name" (node "type_spec"))))
  (under (node "function_declaration")
    (as-scope (this))
    (as-atom public (take "name" (node "function_declaration")))
    (under (node "block")
      (as-scope (this)))
    (under (node "identifier")
      (as-use))))
`
	prog, err := pattern.LoadExtractPack("scope.rft", src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	code := []byte("package p\n\ntype T int\n\nfunc F() {\n\tif true {\n\t\tx := T\n\t}\n}\n")
	if err := os.WriteFile(path, code, 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := ingestutil.ParseSourceFile(t.Context(), ccgo.Engine{}, path, "go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := prog.ExtractErr(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), pf.Root, code, "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(fe.Scopes) != 3 {
		t.Fatalf("scopes=%d want 3: %+v", len(fe.Scopes), fe.Scopes)
	}
	funcIdx := -1
	var blocks []int
	for i, s := range fe.Scopes {
		text := string(code[s.StartByte:s.EndByte])
		if strings.HasPrefix(text, "func ") {
			funcIdx = i
			continue
		}
		blocks = append(blocks, i)
	}
	if funcIdx < 0 || len(blocks) != 2 {
		t.Fatalf("classify scopes func=%d blocks=%v %+v", funcIdx, blocks, fe.Scopes)
	}
	bodyIdx, innerIdx := blocks[0], blocks[1]
	if fe.Scopes[bodyIdx].EndByte-fe.Scopes[bodyIdx].StartByte < fe.Scopes[innerIdx].EndByte-fe.Scopes[innerIdx].StartByte {
		bodyIdx, innerIdx = innerIdx, bodyIdx
	}
	if fe.Scopes[funcIdx].Parent != -1 {
		t.Fatalf("func parent=%d want -1", fe.Scopes[funcIdx].Parent)
	}
	if fe.Scopes[bodyIdx].Parent != funcIdx {
		t.Fatalf("body parent=%d want func %d", fe.Scopes[bodyIdx].Parent, funcIdx)
	}
	if fe.Scopes[innerIdx].Parent != bodyIdx {
		t.Fatalf("inner parent=%d want body %d", fe.Scopes[innerIdx].Parent, bodyIdx)
	}
	var tAtom, fAtom *project.AtomDef
	for i := range fe.Atoms {
		switch fe.Atoms[i].Name {
		case "T":
			tAtom = &fe.Atoms[i]
		case "F":
			fAtom = &fe.Atoms[i]
		}
	}
	if tAtom == nil || fAtom == nil {
		t.Fatalf("atoms=%+v", fe.Atoms)
	}
	if tAtom.ScopeIdx != -1 {
		t.Fatalf("T ScopeIdx=%d want -1 (file)", tAtom.ScopeIdx)
	}
	if fAtom.ScopeIdx != funcIdx {
		t.Fatalf("F ScopeIdx=%d want func %d", fAtom.ScopeIdx, funcIdx)
	}
	var sawX, sawTUse bool
	for _, u := range fe.Usages {
		if u.Name == "x" {
			sawX = true
			if u.ScopeIdx != innerIdx {
				t.Fatalf("use x ScopeIdx=%d want inner %d", u.ScopeIdx, innerIdx)
			}
		}
		if u.Name == "T" {
			sawTUse = true
			if u.ScopeIdx != innerIdx {
				t.Fatalf("use T ScopeIdx=%d want inner %d", u.ScopeIdx, innerIdx)
			}
		}
	}
	if !sawX || !sawTUse {
		t.Fatalf("uses=%+v", fe.Usages)
	}
}

func TestExtractErr_NilProgramOrTree(t *testing.T) {
	fe, err := (*pattern.ExtractProgram)(nil).ExtractErr(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), nil, nil, "x.go")
	if !errors.Is(err, pattern.ErrExtract) || fe != nil {
		t.Fatalf("nil program: fe=%v err=%v", fe, err)
	}
	fe, err = (&pattern.ExtractProgram{}).ExtractErr(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), nil, nil, "x.go")
	if !errors.Is(err, pattern.ErrExtract) || fe != nil {
		t.Fatalf("nil tree: fe=%v err=%v", fe, err)
	}
}
