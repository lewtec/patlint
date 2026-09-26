package pattern

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestParse_InvertRegex(t *testing.T) {
	n, err := ParsePattern(`(capture name (not (regex "^(main|init)$")))`)
	require.NoError(t, err)
	require.False(t, n.Kind != "string" || !n.Invert || n.Regex != "^(main|init)$" || n.As != "name",
		"got kind=%s invert=%v re=%q as=%q", n.Kind, n.Invert, n.Regex, n.As)

	bare, err := ParsePattern(`(not (regex "^(main|init)$"))`)
	require.NoError(t, err)
	require.False(t, bare.Kind != "string" || !bare.Invert || bare.Regex != "^(main|init)$",
		"bare got kind=%s invert=%v re=%q", bare.Kind, bare.Invert, bare.Regex)

	// bare ! remains a lit when not followed by /
	lit, err := ParsePattern(`(seq (token "a") "!=" (token "b"))`)
	require.NoError(t, err)

	_ = lit
}

func TestMatch_InvertNameRegex_ContextBackground(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package main

import "context"

func main() {
	_ = context.Background()
}

func init() {
	_ = context.Background()
}

func Helper() {
	_ = context.Background()
}

func mainHelper() {
	_ = context.Background()
}

func (s *S) Method() {
	_ = context.Background()
}

type S struct{}
`)
	path := lewpath.New(dir, "sample.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(seq (token "func") (capture name (not (regex "^(main|init)$"))) "(" (* any) ")" "{" (* any) (capture c (ref "go:context::Background")) (* any) "}")`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "sample.go", src, pat)
	got := map[string]bool{}
	for _, m := range ms {
		if sites := m.Captures["name"]; len(sites) > 0 {
			got[sites[0].Text(src)] = true
		}
	}
	require.False(t, got["main"] || got["init"],
		"main/init should be excluded: %v", got)
	require.False(t, !got["Helper"] || !got["mainHelper"],
		"want Helper and mainHelper, got %v", got)

	mpat, err := ParsePattern(`(seq (token "func") "(" (* any) ")" (capture name (not (regex "^(main|init)$"))) "(" (* any) ")" "{" (* any) (capture c (ref "go:context::Background")) (* any) "}")`)
	require.NoError(t, err)

	mms := mustMatchFile(t, dir, path, "sample.go", src, mpat)
	require.Len(t, mms, 1,
		"method matches=%d want 1", len(mms))

	name := mms[0].Captures["name"][0].Text(src)
	require.Equal(t, "Method", name,
		"method name=%q", name)

}
