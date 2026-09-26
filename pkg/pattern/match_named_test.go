package pattern

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestNamedRegexCaptures(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc TestFoo(t *testing.T) {}\n")
	path := lewpath.New(dir, "x_test.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(seq (token "func") (capture name (regex "^Test(?P<rest>.*)")))`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "x_test.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d", len(ms))

	if got := ms[0].Captures["name"][0].Text(src); got != "TestFoo" {
		t.Logf("name=%q caps=%v", got, PublicCaptures(ms[0], src))
	}
	rests := ms[0].Captures["rest"]
	require.NotEmpty(t, rests,
		"empty rest")

	rest := rests[0]
	{
		got := rest.Text(src)
		require.Equal(t, "Foo", got,
			"rest=%q want Foo", got)
	}

	got := string(src[rest.StartByte:rest.EndByte])
	require.Equal(t, "Foo", got,
		"rest span text=%q want Foo [%d:%d]", got, rest.StartByte, rest.EndByte)

}

func TestMatchDualRestClosingBrace(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import (
	"context"
	"testing"
)

func TestFoo(t *testing.T) {
	ctx := context.Background()
	_ = ctx
}

func Helper() {
	_ = context.Background()
}
`)
	path := lewpath.New(dir, "x_test.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (capture c (ref "go:context::Background")) (* any) "}")`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "x_test.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d want 1; caps=%v", len(ms), capsOf(ms, src))

	cs := ms[0].Captures["c"]
	require.NotEmpty(t, cs,
		"empty c")

	c := cs[0]
	{
		got := c.Text(src)
		require.Equal(t, "context.Background", got,
			"c=%q want context.Background", got)
	}

	got := string(src[c.StartByte:c.EndByte])
	require.Equal(t, "context.Background", got,
		"c span text=%q", got)

}

func capsOf(ms []Match, src []byte) []map[string]string {
	out := make([]map[string]string, len(ms))
	for i, m := range ms {
		out[i] = PublicCaptures(m, src)
	}
	return out
}
