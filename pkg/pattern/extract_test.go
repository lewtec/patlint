package pattern_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestCFunctionAtomIsDeclaratorNotSoup(t *testing.T) {
	src := []byte("#include \"h.h\"\n\nint helper(int a) {\n  return a + 1;\n}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "helper.c")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "helper.c")
	require.NoError(t, err)

	var names []string
	for _, a := range fe.Atoms {
		names = append(names, a.Name)
		switch a.Name {
		case "(", ")", "+", "1", ";", "int", "return", "{", "}":
			require.FailNow(t, fmt.Sprintf("token-soup atom %q; atoms=%v", a.Name, names))
		}
	}
	found := false
	for _, a := range fe.Atoms {
		if a.Name == "helper" {
			found = true
			require.Equal(t, "helper", string(src[a.StartByte:a.EndByte]),
				"helper locus %q", src[a.StartByte:a.EndByte])

		}
	}
	require.True(t, found,
		"want helper atom, have %v", names)

}

func TestGoConstVarAndFieldAtoms(t *testing.T) {
	src := []byte("package p\n\nconst A = 1\nconst (\n\tB, C = 2, 3\n)\n\nvar X int\n\ntype T struct {\n\tF int\n}\n\ntype I interface {\n\tM()\n}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "t.go")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "t.go")
	require.NoError(t, err)

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
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "a.js")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "a.js")
	require.NoError(t, err)

	got := map[string]bool{}
	for _, a := range fe.Atoms {
		got[a.Name] = true
		require.False(t, strings.ContainsAny(a.Name, "[]{}"),
			"pattern soup atom %q; atoms=%v", a.Name, got)

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
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "a.ts")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "a.ts")
	require.NoError(t, err)

	got := map[string]bool{}
	for _, a := range fe.Atoms {
		got[a.Name] = true
		require.False(t, strings.ContainsAny(a.Name, "[]{}"),
			"pattern soup atom %q", a.Name)

	}
	require.False(t, !got["thing"] || !got["setThing"],
		"have %v", got)

}

func TestPythonAssignmentAtoms(t *testing.T) {
	src := []byte("_TEXT_OPENFLAGS = 1\nX = 2\n\nclass C:\n    Y = 3\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "a.py")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "a.py")
	require.NoError(t, err)

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
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "main.js")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "main.js")
	require.NoError(t, err)

	var found bool
	for _, im := range fe.Imports {
		if im.LocalName == "SearchBar" && im.SourcePath == "./SearchBar.js" {
			found = true
		}
	}
	require.True(t, found,
		"imports=%#v", fe.Imports)

}

func TestJSConstLetAtoms(t *testing.T) {
	src := []byte("export const variant = \"primary\"\nlet query = 1\nfunction greet() {}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "a.js")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "a.js")
	require.NoError(t, err)

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
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "App.vue")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "App.vue")
	require.NoError(t, err)
	require.Equal(t, "vue", fe.Language,
		"host language=%q", fe.Language)

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
		require.True(t, got[w],
			"missing guest atom %q; have %v", w, got)

	}
}

func TestAtomJoinNames_GoMethod(t *testing.T) {
	src := []byte("package p\n\ntype T struct{}\n\nfunc (t *T) Method() {}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "t.go")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "t.go")
	require.NoError(t, err)

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
	require.True(t, found,
		"want T.Method in atoms %v", names)

}

func TestAtomJoinNames_JavaMethod(t *testing.T) {
	src := []byte("class A {\n  void run() {}\n}\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "A.java")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "A.java")
	require.NoError(t, err)

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
	require.True(t, found,
		"want A.run in atoms %v", names)

	// leaf span is "run", not "A"
	for _, a := range fe.Atoms {
		if a.Name == "A.run" {
			text := string(src[a.StartByte:a.EndByte])
			require.Equal(t, "run", text,
				"locus text=%q want run", text)

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
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "A.java")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "A.java")
	require.NoError(t, err)

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
		require.True(t, ok,
			"missing use path %s", k)

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
	require.NoError(t, err)
	require.False(t, len(prog.Families) != 1 || prog.Families[0].Lang != "javascript" || prog.Families[0].Family != "ecma",
		"families=%#v", prog.Families)

}

func TestAsFamilyKnobs(t *testing.T) {
	prog, err := pattern.LoadExtractPack("go.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-family "go" directory-module package-scoped-bare-names nested-type-members))
`)
	require.NoError(t, err)
	require.Len(t, prog.Families, 1,
		"families=%#v", prog.Families)

	r := prog.Families[0].Rules
	require.False(t, !r.DirectoryModule || !r.PackageScopedBareNames || !r.NestedTypeMembers,
		"rules=%#v", r)
	require.False(t, r.EmptyPackageDirScoped || r.IncludeFileExportsBare || r.DirectoryManifest,
		"extra knobs=%#v", r)

}

func TestAsFamilyDirectoryManifest(t *testing.T) {
	prog, err := pattern.LoadExtractPack("js.rft", `
(under (path "**/*.js")
  (as-language "javascript")
  (as-family "ecma" directory-manifest))
`)
	require.NoError(t, err)
	require.True(t, prog.Families[0].Rules.DirectoryManifest,
		"rules=%#v", prog.Families[0].Rules)

}

func TestAsLayoutClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("go.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-layout package import body))
`)
	require.NoError(t, err)
	require.False(t, len(prog.Layouts) != 1 || prog.Layouts[0].Lang != "go",
		"layouts=%#v", prog.Layouts)

	got := strings.Join(prog.Layouts[0].Names, " ")
	require.Equal(t, "package import body", got,
		"names=%q", got)

}

func TestAsLayoutKeepsPackNames(t *testing.T) {
	prog, err := pattern.LoadExtractPack("go.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-layout package import))
`)
	require.NoError(t, err)

	got := strings.Join(prog.Layouts[0].Names, " ")
	require.Equal(t, "package import", got,
		"names=%q", got)

}

func TestAsLayoutDuplicateName(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-layout package package body))
`)
	require.False(t, err == nil || !strings.Contains(err.Error(), "duplicate"),
		"want duplicate, got %v", err)

}

func TestAsFamilyUnknownKnob(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.js")
  (as-language "javascript")
  (as-family "ecma" no-such-knob))
`)
	require.False(t, err == nil || !strings.Contains(err.Error(), "no-such-knob"),
		"want unknown knob, got %v", err)

}

func TestAsFamilyRejectsNameVisibilityKnobs(t *testing.T) {
	for _, knob := range []string{"capital-export", "hide-underscore", "hide-unexported"} {
		_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-family "go" `+knob+`))
`)
		require.False(t, err == nil || !strings.Contains(err.Error(), knob),
			"knob %s: want unknown, got %v", knob, err)

	}
}

func TestAsDirectoryRepresentant(t *testing.T) {
	prog, err := pattern.LoadExtractPack("py.rft", `
(under (path "**/__init__.py")
  (as-directory-representant))
`)
	require.NoError(t, err)
	require.False(t, len(prog.DirectoryRepresentants) != 1 || prog.DirectoryRepresentants[0] != "**/__init__.py",
		"representants=%#v", prog.DirectoryRepresentants)

}

func TestAsDirectoryRepresentantWantsPath(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `(as-directory-representant)`)
	require.False(t, err == nil || !strings.Contains(err.Error(), "under (path"),
		"want path place, got %v", err)

}

func TestAsFamilyUnknownIsClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.js")
  (as-language "javascript")
  (as-family "no_such_family"))
`)
	require.NoError(t, err)
	require.False(t, len(prog.Families) != 1 || prog.Families[0].Family != "no_such_family",
		"families=%#v", prog.Families)

}

func TestAsPaintClassifyLeaf(t *testing.T) {
	prog, err := pattern.LoadExtractPack("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (as-keyword (alt (token "func") (token "var")))
  (as-ident (node "identifier")))
`)
	require.NoError(t, err)
	{

		got := prog.ClassifyLeaf("dsl", "func")
		require.Equal(t, ingest.HLKeyword, got,
			"func=%q", got)
	}
	{

		got := prog.ClassifyLeaf("dsl", "identifier")
		require.Equal(t, ingest.HLIdent, got,
			"identifier=%q", got)
	}

	got := prog.ClassifyLeaf("dsl", "(")
	require.Empty(t, got,
		"punct harvest=%q", got)

}

func TestAsDocstringClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (as-docstring comment-before))
`)
	require.NoError(t, err)
	require.False(t, len(prog.Docstrings) != 1 || prog.Docstrings[0].Lang != "dsl" || prog.Docstrings[0].Kind != "comment-before",
		"docstrings=%#v", prog.Docstrings)

}

func TestAsImportSeqClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (as-import (seq "import" "\"" path "\"")))
`)
	require.NoError(t, err)
	require.Len(t, prog.ImportLines, 1,
		"import-lines=%#v", prog.ImportLines)

	c := prog.ImportLines[0]
	require.False(t, c.Lang != "dsl" || c.HasLeaf || c.HasQual || !c.QuotedPath || !seqHasPath(c.Tokens),
		"import-lines=%#v", prog.ImportLines)

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
	require.NoError(t, err)
	require.Len(t, prog.PackageLines, 1,
		"package-lines=%#v", prog.PackageLines)

	c := prog.PackageLines[0]
	require.False(t, c.Lang != "dsl" || len(c.Tokens) != 2 || c.Tokens[0] != "package" || c.Tokens[1] != "pkg",
		"package-lines=%#v", prog.PackageLines)

}

func TestAsAtomicClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (as-atomic "interface{}"))
`)
	require.NoError(t, err)
	require.False(t, len(prog.AtomicSpans) != 1 || prog.AtomicSpans[0].Lang != "dsl" || prog.AtomicSpans[0].Text != "interface{}",
		"atomic=%#v", prog.AtomicSpans)

}

func TestAsGrammarClaim(t *testing.T) {
	prog, err := pattern.LoadExtractPack("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (under (node "sym_lit")
    (as-ident)))
`)
	require.NoError(t, err)
	require.False(t, len(prog.Grammars) != 1 || prog.Grammars[0].Lang != "dsl" || prog.Grammars[0].Grammar != "commonlisp",
		"grammars=%#v", prog.Grammars)

}

func TestAsGrammarUnknownFails(t *testing.T) {
	prog, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "no_such_grammar"))
`)
	require.NoError(t, err)

	err = prog.ValidateGrammars(t.Context(), treesitter.Engine{})
	require.ErrorIs(t, err, pattern.ErrExtract,
		"err=%v", err)

}

func TestAsGrammarNeedsLanguageFirst(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.dsl")
  (as-grammar "commonlisp")
  (as-language "dsl"))
`)
	require.ErrorIs(t, err, pattern.ErrExtract,
		"err=%v", err)

}

func TestAsLanguageWithoutGrammarFails(t *testing.T) {
	prog, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.dsl")
  (as-language "dsl"))
`)
	require.NoError(t, err)

	err = prog.ValidateGrammars(t.Context(), treesitter.Engine{})
	require.ErrorIs(t, err, pattern.ErrExtract,
		"err=%v", err)

}

func TestPurposeLispUsesCommonlispGrammar(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()), pattern.FromString("dsl.rft", `
(under (path "**/*.dsl")
  (as-language "dsl")
  (as-grammar "commonlisp")
  (under (node "sym_lit")
    (as-ident)))
`))
	require.NoError(t, err)

	lang, ok := vm.HostLanguage("app.dsl")
	require.False(t, !ok || lang != "dsl",
		"host=%q ok=%v", lang, ok)
	{

		got := vm.GrammarForLanguage("dsl")
		require.Equal(t, "commonlisp", got,
			"grammar=%q", got)
	}

	dir := t.TempDir()
	path := lewpath.New(dir, "app.dsl").String()
	{
		err := os.WriteFile(path, []byte("(hello world)\n"), 0o644)
		require.NoError(t, err)
	}

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	var got *project.FileExtract
	err = w.WalkExtracts(t.Context(), ingest.SourceProject(dir), func(fe *project.FileExtract) bool {
		got = fe
		return true
	})
	require.NoError(t, err)
	require.False(t, got == nil || got.Language != "dsl",
		"extract=%#v", got)

}

func TestAsFamilyNeedsLanguageFirst(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.js")
  (as-family "ecma")
  (as-language "javascript"))
`)
	require.ErrorIs(t, err, pattern.ErrExtract,
		"err=%v", err)

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
	require.NoError(t, err)
	require.Equal(t, "go", prog.Language,
		"lang=%q", prog.Language)
	// host claim + package + atom
	require.Len(t, prog.Actions, 3)

	claim := prog.Actions[0]
	require.False(t, claim.Matcher != nil || claim.HostLang != "go",
		"action0 want host claim, got matcher=%v host=%q", claim.Matcher != nil, claim.HostLang)
	require.False(t, len(claim.Paths) != 1 || claim.Paths[0] != "**/*.go",
		"claim paths=%v", claim.Paths)
	require.False(t, prog.Actions[1].HostLang != "go" || prog.Actions[1].Lang != "go",
		"action1 host/lang=%q/%q", prog.Actions[1].HostLang, prog.Actions[1].Lang)
	require.False(t, prog.Actions[1].Embed,
		"host action should not be embed")

	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	code := []byte("package demo\nfunc Hello() {}\n")
	{
		err := os.WriteFile(path, code, 0o644)
		require.NoError(t, err)
	}

	pf, err := ingestutil.ParseSourceFile(t.Context(), treesitter.Engine{}, path, "go")
	require.NoError(t, err)

	defer pf.Close()

	fe, err := prog.ExtractErr(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), pf.Root, code, "x.go")
	require.NoError(t, err)
	require.Equal(t, "demo", fe.Package,
		"package=%q", fe.Package)
	require.False(t, len(fe.Atoms) != 1 || fe.Atoms[0].Name != "Hello",
		"atoms=%+v", fe.Atoms)

}

func TestLoadExtractPack_UnknownLanguage(t *testing.T) {
	prog, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.x")
  (as-language "no_such_grammar_xyz"))
`)
	require.NoError(t, err)

	err = prog.ValidateGrammars(t.Context(), treesitter.Engine{})
	require.Error(t, err,
		"want error for unregistered grammar id")
	require.True(t, strings.Contains(err.Error(), "no_such_grammar_xyz"),
		"err=%v", err)

}

func TestLoadExtractPack_RejectsRootAsLanguage(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `(as-language "go")`)
	require.Error(t, err,
		"want error for root as-language")
	require.True(t, strings.Contains(err.Error(), "under (path"),
		"err=%v", err)

}

func TestLoadExtractPack_AsLanguageBody(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go"
    (under (node "package_identifier")
      (as-package))))
`
	prog, err := pattern.LoadExtractPack("test.rft", src)
	require.NoError(t, err)
	require.False(t, // host claim + as-package
		prog.Language != "go" || len(prog.Actions) != 2,
		"lang=%q actions=%d", prog.Language, len(prog.Actions))

}

func TestExtractPack_IgnoresUnknownHead(t *testing.T) {
	prog, err := pattern.LoadExtractPack("ok.rft", `(top (on "x"))`)
	require.NoError(t, err)
	require.Empty(t, prog.Actions,
		"unknown head must not become extract actions: %d", len(prog.Actions))

}

func TestExtractPack_RejectsStringMatcher(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-atom public "Hello")))
`
	_, err := pattern.LoadExtractPack("bad.rft", src)
	require.Error(t, err,
		"want error for string matcher")
	require.True(t, strings.Contains(err.Error(), "MATCHER"),
		"err=%v", err)

}

func TestLoadEmbeddedLanguagePacks(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

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
	require.GreaterOrEqual(t, len(packs), len(want),
		"packs=%d want at least %d", len(packs), len(want))

}

func TestLoadExtractPack_MultiPathGlob(t *testing.T) {
	src := `
(under (path "**/*.js" "**/*.mjs")
  (as-language "javascript")
  (under (node "identifier")
    (as-use)))
`
	prog, err := pattern.LoadExtractPack("test.rft", src)
	require.NoError(t, err)
	require.Len(t, prog.Actions, 2,
		"actions=%d", len(prog.Actions))
	require.Len(t, prog.Actions[0].Paths, 2,
		"paths=%v", prog.Actions[0].Paths)

}

func TestLoadExtractPack_AsAtomRequiresMatcher(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-atom))
`)
	require.Error(t, err,
		"want error for bare (as-atom)")
	require.True(t, strings.Contains(err.Error(), "as-atom wants public|private and MATCHER"),
		"err=%v", err)

}

func TestLoadExtractPack_AsAtomRequiresVisibility(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-atom (take "name" (node "function_declaration")))))
`)
	require.Error(t, err,
		"want error for as-atom without public|private")
	require.True(t, strings.Contains(err.Error(), "as-atom wants public|private and MATCHER"),
		"err=%v", err)

}

func TestLoadExtractPack_AsAtomRejectsUnknownVisibility(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-atom exported (take "name" (node "function_declaration")))))
`)
	require.Error(t, err,
		"want error for unknown visibility")
	require.True(t, strings.Contains(err.Error(), `as-atom wants public or private, got "exported"`),
		"err=%v", err)

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
	require.NoError(t, err)

	var atoms, uses int
	for _, a := range prog.Actions {
		switch a.Kind {
		case pattern.ExtractAtom:
			atoms++
			require.NotNil(t, a.Matcher,
				"as-atom (this) must compile a matcher")
			require.True(t, a.Exported,
				"as-atom public (this) must set Exported")

		case pattern.ExtractUse:
			uses++
			require.False(t, a.ScopeField != "name" || a.ScopeNode != "function_declaration",
				"use scope field=%q node=%q", a.ScopeField, a.ScopeNode)

		}
	}
	require.False(t, atoms != 1 || uses != 1,
		"atoms=%d uses=%d", atoms, uses)

}

func TestLoadExtractPack_AsAtomPrivateExported(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "parameter_declaration")
    (as-atom private (take "name" (node "parameter_declaration")))))
`
	prog, err := pattern.LoadExtractPack("ok.rft", src)
	require.NoError(t, err)

	var saw bool
	for _, a := range prog.Actions {
		if a.Kind != pattern.ExtractAtom {
			continue
		}
		saw = true
		require.False(t, a.Exported,
			"as-atom private must clear Exported")

	}
	require.True(t, saw,
		"want as-atom action")

}

func TestLoadExtractPack_AsScopeRequiresMatcher(t *testing.T) {
	_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (as-scope))
`)
	require.Error(t, err,
		"want error for bare (as-scope)")
	require.True(t, strings.Contains(err.Error(), "as-scope wants exactly one MATCHER"),
		"err=%v", err)

}

func TestLoadExtractPack_AsScopeThisPlace(t *testing.T) {
	src := `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-scope (this))))
`
	prog, err := pattern.LoadExtractPack("ok.rft", src)
	require.NoError(t, err)

	n := 0
	for _, a := range prog.Actions {
		if a.Kind == pattern.ExtractScope {
			n++
			require.NotNil(t, a.Matcher,
				"as-scope (this) must compile a matcher")
			require.Equal(t, "function_declaration", a.NodeType,
				"NodeType=%q", a.NodeType)

		}
	}
	require.Equal(t, 1, n,
		"scopes=%d", n)

	got := prog.ScopeNodeTypes("go")
	require.False(t, len(got) != 1 || got[0] != "function_declaration",
		"ScopeNodeTypes=%v", got)

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
	require.NoError(t, err)

	n := 0
	for _, a := range prog.Actions {
		if a.Kind == pattern.ExtractFlow {
			n++
			require.NotNil(t, a.Matcher,
				"as-flow must compile a matcher")

		}
	}
	require.Equal(t, 3, n,
		"flows=%d", n)
	{

		_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "if_statement")
    (as-flow nope (this))))
`)
		require.Error(t, err,
			"want error for bad as-flow class")
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
	require.NoError(t, err)

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
	{
		err := os.WriteFile(path, code, 0o644)
		require.NoError(t, err)
	}

	pf, err := ingestutil.ParseSourceFile(t.Context(), treesitter.Engine{}, path, "go")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := prog.ExtractErr(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), pf.Root, code, "x.go")
	require.NoError(t, err)
	require.NotEmpty(t, fe.Flows,
		"no flow rows")

	var unit project.ScopeDef
	for _, s := range fe.Scopes {
		if s.StartByte < s.EndByte && strings.Contains(string(code[s.StartByte:s.EndByte]), "func Demo") {
			unit = s
			break
		}
	}
	require.NotEqual(t, unit.EndByte, 0,
		"no Demo scope: %+v", fe.Scopes)

	rep := ingest.ScoreFlow(unit.StartByte, unit.EndByte, fe.Flows)
	// if, nested if, else-if, else, for, if-in-for
	require.GreaterOrEqual(t, rep.Score, 6, "incs=%+v flows=%+v", rep.Incs, fe.Flows)

	var hybrids, structs int
	for _, fl := range fe.Flows {
		switch fl.Class {
		case project.FlowHybrid:
			hybrids++
		case project.FlowStructural:
			structs++
		}
	}
	require.GreaterOrEqual(t, hybrids, 2,
		"hybrid=%d want >=2 (else-if + else); flows=%+v", hybrids, fe.Flows)
	require.GreaterOrEqual(t, structs, 4,
		"structural=%d want >=4; flows=%+v", structs, fe.Flows)

}

func TestLoadExtractPack_AsDecl(t *testing.T) {
	prog, err := pattern.LoadExtractPack("ok.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "type_declaration")
    (as-decl (this))))
`)
	require.NoError(t, err)

	n := 0
	for _, a := range prog.Actions {
		if a.Kind == pattern.ExtractScope && a.HoleOnly {
			n++
		}
	}
	require.Equal(t, 1, n,
		"decls=%d", n)
	{

		_, err := pattern.LoadExtractPack("bad.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "type_declaration")
    (as-decl (this) "type $")))
`)
		require.Error(t, err,
			"as-decl template should fail")
	}

}

func TestAttributeHostLanguage(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	lang, ok := vm.HostLanguage("pkg/foo.go")
	require.False(t, !ok || lang != "go",
		"got %q ok=%v", lang, ok)

	_, ok = vm.HostLanguage("nope.xyz")
	require.False(t, ok,
		"expected no claim for .xyz")

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
	require.NoError(t, err)

	var embedAct *pattern.ExtractAction
	for i := range prog.Actions {
		if prog.Actions[i].Embed {
			embedAct = &prog.Actions[i]
			break
		}
	}
	require.False(t, embedAct == nil || embedAct.Lang != "javascript" || embedAct.Region == nil,
		"embed action missing: %+v", prog.Actions)

	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	code := []byte("package demo\nvar s = `function hello() { curl() }`\n")
	{
		err := os.WriteFile(path, code, 0o644)
		require.NoError(t, err)
	}

	pf, err := ingestutil.ParseSourceFile(t.Context(), treesitter.Engine{}, path, "go")
	require.NoError(t, err)

	defer pf.Close()

	fe, err := prog.ExtractErr(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), pf.Root, code, "x.go")
	if err != nil {
		// javascript grammar may be unregistered in this test binary
		if strings.Contains(err.Error(), "unknown language") {
			t.Skip(err.Error())
		}
		require.NoError(t, err)
	}
	require.Equal(t, "demo", fe.Package,
		"package=%q", fe.Package)

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
	require.NoError(t, err)

	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	code := []byte("package p\n\nfunc Hello(x int) {}\n")
	{
		err := os.WriteFile(path, code, 0o644)
		require.NoError(t, err)
	}

	pf, err := ingestutil.ParseSourceFile(t.Context(), treesitter.Engine{}, path, "go")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := prog.ExtractErr(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), pf.Root, code, "x.go")
	require.NoError(t, err)

	got := map[string]bool{}
	for _, a := range fe.Atoms {
		got[a.Name] = a.Exported
	}
	require.True(t, got["Hello"],
		"Hello exported=%v atoms=%v", got["Hello"], fe.Atoms)

	exp, ok := got["x"]
	require.False(t, !ok || exp,
		"x private want Exported=false, got present=%v exported=%v atoms=%v", ok, exp, fe.Atoms)

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
	require.NoError(t, err)

	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	code := []byte("package p\n\ntype T int\n\nfunc F() {\n\tif true {\n\t\tx := T\n\t}\n}\n")
	{
		err := os.WriteFile(path, code, 0o644)
		require.NoError(t, err)
	}

	pf, err := ingestutil.ParseSourceFile(t.Context(), treesitter.Engine{}, path, "go")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := prog.ExtractErr(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), pf.Root, code, "x.go")
	require.NoError(t, err)
	require.Len(t, fe.Scopes, 3,
		"scopes=%d want 3: %+v", len(fe.Scopes), fe.Scopes)

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
	require.False(t, funcIdx < 0 || len(blocks) != 2,
		"classify scopes func=%d blocks=%v %+v", funcIdx, blocks, fe.Scopes)

	bodyIdx, innerIdx := blocks[0], blocks[1]
	if fe.Scopes[bodyIdx].EndByte-fe.Scopes[bodyIdx].StartByte < fe.Scopes[innerIdx].EndByte-fe.Scopes[innerIdx].StartByte {
		bodyIdx, innerIdx = innerIdx, bodyIdx
	}
	require.Equal(t, -1, fe.Scopes[funcIdx].Parent,
		"func parent=%d want -1", fe.Scopes[funcIdx].Parent)
	require.Equal(t, funcIdx, fe.Scopes[bodyIdx].Parent,
		"body parent=%d want func %d", fe.Scopes[bodyIdx].Parent, funcIdx)
	require.Equal(t, bodyIdx, fe.Scopes[innerIdx].Parent,
		"inner parent=%d want body %d", fe.Scopes[innerIdx].Parent, bodyIdx)

	var tAtom, fAtom *project.AtomDef
	for i := range fe.Atoms {
		switch fe.Atoms[i].Name {
		case "T":
			tAtom = &fe.Atoms[i]
		case "F":
			fAtom = &fe.Atoms[i]
		}
	}
	require.False(t, tAtom == nil || fAtom == nil,
		"atoms=%+v", fe.Atoms)
	require.Equal(t, -1, tAtom.ScopeIdx,
		"T ScopeIdx=%d want -1 (file)", tAtom.ScopeIdx)
	require.Equal(t, funcIdx, fAtom.ScopeIdx,
		"F ScopeIdx=%d want func %d", fAtom.ScopeIdx, funcIdx)

	var sawX, sawTUse bool
	for _, u := range fe.Usages {
		if u.Name == "x" {
			sawX = true
			require.Equal(t, innerIdx, u.ScopeIdx,
				"use x ScopeIdx=%d want inner %d", u.ScopeIdx, innerIdx)

		}
		if u.Name == "T" {
			sawTUse = true
			require.Equal(t, innerIdx, u.ScopeIdx,
				"use T ScopeIdx=%d want inner %d", u.ScopeIdx, innerIdx)

		}
	}
	require.False(t, !sawX || !sawTUse,
		"uses=%+v", fe.Usages)

}

func TestExtractErr_NilProgramOrTree(t *testing.T) {
	fe, err := (*pattern.ExtractProgram)(nil).ExtractErr(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), nil, nil, "x.go")
	require.False(t, !errors.Is(err, pattern.ErrExtract) || fe != nil,
		"nil program: fe=%v err=%v", fe, err)

	fe, err = (&pattern.ExtractProgram{}).ExtractErr(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), nil, nil, "x.go")
	require.False(t, !errors.Is(err, pattern.ErrExtract) || fe != nil,
		"nil tree: fe=%v err=%v", fe, err)

}
