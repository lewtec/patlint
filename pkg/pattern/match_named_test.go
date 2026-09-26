package pattern

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestNamedRegexCaptures(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc TestFoo(t *testing.T) {}\n")
	path := lewpath.New(dir, "x_test.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	pat, err := ParsePattern(`(seq (token "func") (capture name (regex "^Test(?P<rest>.*)")))`)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "x_test.go", src, pat)
	if len(ms) != 1 {
		t.Fatalf("matches=%d", len(ms))
	}
	if got := ms[0].Captures["name"][0].Text(src); got != "TestFoo" {
		t.Logf("name=%q caps=%v", got, PublicCaptures(ms[0], src))
	}
	rests := ms[0].Captures["rest"]
	if len(rests) == 0 {
		t.Fatal("empty rest")
	}
	rest := rests[0]
	if got := rest.Text(src); got != "Foo" {
		t.Fatalf("rest=%q want Foo", got)
	}
	if got := string(src[rest.StartByte:rest.EndByte]); got != "Foo" {
		t.Fatalf("rest span text=%q want Foo [%d:%d]", got, rest.StartByte, rest.EndByte)
	}
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
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	pat, err := ParsePattern(`(seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (capture c (ref "go:context::Background")) (* any) "}")`)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "x_test.go", src, pat)
	if len(ms) != 1 {
		t.Fatalf("matches=%d want 1; caps=%v", len(ms), capsOf(ms, src))
	}
	cs := ms[0].Captures["c"]
	if len(cs) == 0 {
		t.Fatal("empty c")
	}
	c := cs[0]
	if got := c.Text(src); got != "context.Background" {
		t.Fatalf("c=%q want context.Background", got)
	}
	if got := string(src[c.StartByte:c.EndByte]); got != "context.Background" {
		t.Fatalf("c span text=%q", got)
	}
}

func capsOf(ms []Match, src []byte) []map[string]string {
	out := make([]map[string]string, len(ms))
	for i, m := range ms {
		out[i] = PublicCaptures(m, src)
	}
	return out
}
