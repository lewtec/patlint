package pattern

import (
	"encoding/json"
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingestutil"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestSexpTextAndJSONSameMatches(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) {}\n")
	path := lewpath.New(dir, "x.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	text, err := ParseToPat(`(token "interface{}")`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(patToJSON(text))
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := DecodePatJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if FormatSexp(text) != FormatSexp(fromJSON) {
		t.Fatalf("sexpr round-trip diverged:\n  text %s\n  json %s", FormatSexp(text), FormatSexp(fromJSON))
	}

	msText := mustMatchFile(t, dir, path, "x.go", src, mustPatToNode(t, text))
	msJSON := mustMatchFile(t, dir, path, "x.go", src, mustPatToNode(t, fromJSON))
	if len(msText) != len(msJSON) || len(msText) == 0 {
		t.Fatalf("text matches=%d json=%d", len(msText), len(msJSON))
	}

	// Direct MatchFilePat (no Node in match path)
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	msPat, err := matchFilePatPol(testSess(), dir, "x.go", src, mustParseRoot(t, path), text, nil, vm.TapePolicy("x.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(msPat) != len(msText) {
		t.Fatalf("MatchFilePat=%d want %d", len(msPat), len(msText))
	}
}

func TestCompilePatVsLegacyNode(t *testing.T) {
	// Both entry points compile Core Pat NFA (no Pat→Node→NFA detour).
	for _, s := range []string{
		`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) "," (capture ERR any) ")")`,
		`(* (unify c (ref "go:errors::New")))`,
		`(seq (token "if") (unify a any) ">" (unify b any) "{" (token "return") (unify a any) "}")`,
	} {
		t.Run(s, func(t *testing.T) {
			n, err := ParsePattern(s)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := compilePattern(n); err != nil {
				t.Fatal(err)
			}
			p, err := ParseToPat(s)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := CompilePat(p); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func mustPatToNode(t *testing.T, p Pat) Node {
	t.Helper()
	n, err := PatToNode(p)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func mustParseRoot(t *testing.T, abs string) *sitter.Node {
	t.Helper()
	pf, err := ingestutil.ParseSourceFile(t.Context(), ccgo.Engine{}, abs, "go")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pf.Close() })
	return pf.Root
}
