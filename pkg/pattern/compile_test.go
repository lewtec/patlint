package pattern

import (
	"encoding/json"
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestSexpTextAndJSONSameMatches(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) {}\n")
	path := lewpath.New(dir, "x.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	text, err := ParseToPat(`(token "interface{}")`)
	require.NoError(t, err)

	raw, err := json.Marshal(patToJSON(text))
	require.NoError(t, err)

	fromJSON, err := DecodePatJSON(raw)
	require.NoError(t, err)
	require.Equal(t, FormatSexp(fromJSON), FormatSexp(text),
		"sexpr round-trip diverged:\n  text %s\n  json %s", FormatSexp(text), FormatSexp(fromJSON))

	msText := mustMatchFile(t, dir, path, "x.go", src, mustPatToNode(t, text))
	msJSON := mustMatchFile(t, dir, path, "x.go", src, mustPatToNode(t, fromJSON))
	require.False(t, len(msText) != len(msJSON) || len(msText) == 0,
		"text matches=%d json=%d", len(msText), len(msJSON))

	// Direct MatchFilePat (no Node in match path)
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	msPat, err := matchFilePatPol(testSess(), dir, "x.go", src, mustParseRoot(t, path), text, nil, vm.TapePolicy("x.go"))
	require.NoError(t, err)
	require.Len(t, msPat, len(msText),
		"MatchFilePat=%d want %d", len(msPat), len(msText))

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
			require.NoError(t, err)
			{

				_, err := compilePattern(n)
				require.NoError(t, err)
			}

			p, err := ParseToPat(s)
			require.NoError(t, err)
			{

				_, err := CompilePat(p)
				require.NoError(t, err)
			}

		})
	}
}

func mustPatToNode(t *testing.T, p Pat) Node {
	t.Helper()
	n, err := PatToNode(p)
	require.NoError(t, err)

	return n
}

func mustParseRoot(t *testing.T, abs string) *sitter.Node {
	t.Helper()
	pf, err := ingestutil.ParseSourceFile(t.Context(), treesitter.Engine{}, abs, "go")
	require.NoError(t, err)

	t.Cleanup(func() { pf.Close() })
	return pf.Root
}
