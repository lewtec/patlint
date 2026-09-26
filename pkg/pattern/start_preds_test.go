package pattern

import (
	"testing"
)

func parsePat(t *testing.T, s string) Pat {
	t.Helper()
	core, err := ParseToPat(s)
	if err != nil {
		t.Fatal(err)
	}
	return core
}

func TestAnalyzeStartPreds_lit(t *testing.T) {
	n, err := compilePat(parsePat(t, `(token "interface{}")`))
	if err != nil {
		t.Fatal(err)
	}
	if len(n.startPreds) != 1 {
		t.Fatalf("startPreds=%+v", n.startPreds)
	}
	if n.startPreds[0].kind != predToken && n.startPreds[0].kind != predLit {
		t.Fatalf("kind %v", n.startPreds[0].kind)
	}
	if n.startPreds[0].text != "interface{}" {
		t.Fatalf("text %q", n.startPreds[0].text)
	}
}

func TestAnalyzeStartPreds_anyDisables(t *testing.T) {
	// Rest / any first atom must not filter (would miss valid starts).
	n, err := compilePat(parsePat(t, `(seq (* any) (token "interface{}"))`))
	if err != nil {
		t.Fatal(err)
	}
	if n.startPreds != nil {
		t.Fatalf("want no filter for leading rest, got %+v", n.startPreds)
	}
}

func TestAnalyzeStartPreds_discardedBlank(t *testing.T) {
	op, err := LoadOp("../../testdata/pattern/go_discarded_error_blank")
	if err != nil {
		t.Fatal(err)
	}
	core, err := op.CorePat()
	if err != nil {
		t.Fatal(err)
	}
	n, err := compilePat(core)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.startPreds) == 0 {
		t.Fatal("want start filter on discarded-error pattern")
	}
	// First atom is (capture blank (regex "^_$")) → regex
	sawRegex := false
	for _, p := range n.startPreds {
		if p.kind == predRegex {
			sawRegex = true
		}
	}
	if !sawRegex {
		t.Fatalf("want regex first pred, got %+v", n.startPreds)
	}
}
