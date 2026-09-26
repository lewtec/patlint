package pattern

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestParseGroupAlt(t *testing.T) {
	n, err := ParsePattern(`(group (alt (seq (token "return") (unify b any)) (seq (token "else") "{" (token "return") (unify b any) "}")))`)
	if err != nil {
		t.Fatal(err)
	}
	// top-level is the group
	g := n
	if g.Kind != "group" {
		// might be seq of one
		if g.Kind == "seq" && len(g.Args) == 1 {
			g = g.Args[0]
		}
	}
	if g.Kind != "group" {
		t.Fatalf("kind=%s ir=%s", n.Kind, mustJSON(n))
	}
	if g.As != "_" {
		t.Fatalf("as=%q", g.As)
	}
	if len(g.Args) != 2 {
		t.Fatalf("arms=%d ir=%s", len(g.Args), mustJSON(g))
	}

}

func TestGroupAltMatchBothShapes(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

func preferX(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func preferXElse(x, y int) int {
	if x > y {
		return x
	} else {
		return y
	}
}

func preferY(x, y int) int {
	if x > y {
		return y
	}
	return x
}
`)
	path := lewpath.New(dir, "m.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	pat, err := ParsePattern(`(seq (token "if") (unify a any) (capture op (regex ">=?")) (unify b any) "{" (token "return") (unify a any) "}" (group (alt (seq (token "return") (unify b any)) (seq (token "else") "{" (token "return") (unify b any) "}"))))`)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "m.go", src, pat)
	if len(ms) != 2 {
		t.Fatalf("matches=%d want 2 (preferX + preferXElse); caps=%v", len(ms), capsOf(ms, src))
	}
	for _, m := range ms {
		if m.Captures["a"][0].Text(src) != "x" || m.Captures["b"][0].Text(src) != "y" {
			t.Fatalf("caps=%v", PublicCaptures(m, src))
		}
		if op := m.Captures["op"][0].Text(src); op != ">" {
			t.Fatalf("op=%q", op)
		}
	}
}

func TestGroupAltNamedCapture(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc f(x, y int) int { return x + y }\nfunc g(x, y int) int { return x - y }\n")
	path := lewpath.New(dir, "m.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	pat, err := ParsePattern(`(seq (token "return") (capture e (group (alt (seq (unify a any) "+" (unify b any)) (seq (unify a any) "-" (unify b any))))))`)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "m.go", src, pat)
	if len(ms) != 2 {
		t.Fatalf("matches=%d want 2; %v", len(ms), capsOf(ms, src))
	}
}
