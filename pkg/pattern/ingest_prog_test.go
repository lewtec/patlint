package pattern_test

import (
	"testing"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/store"
)

func TestIngestClauses_AsAtom(t *testing.T) {
	prog, err := pattern.LoadExtractPack("ok.rft", `
(under (path "**/*.go")
  (as-language "go")
  (under (node "function_declaration")
    (as-scope (this))
    (as-atom public (take "name" (node "function_declaration")))))
`)
	if err != nil {
		t.Fatal(err)
	}
	cl := prog.IngestClauses("x.go")
	if len(cl) != 2 {
		t.Fatalf("clauses=%d", len(cl))
	}
	var sawAtom, sawScope bool
	for _, c := range cl {
		if c.Head.Rel == store.RelationAtom {
			sawAtom = true
			if len(c.Body) != 1 || c.Body[0].Rel != "$as" {
				t.Fatalf("atom body: %+v", c.Body)
			}
		}
		if c.Head.Rel == store.RelationScope {
			sawScope = true
		}
	}
	if !sawAtom || !sawScope {
		t.Fatalf("want atom+scope, got %+v", cl)
	}
	if n := len(prog.IngestClauses("x.js")); n != 0 {
		t.Fatalf("js path should skip go actions, got %d", n)
	}
}
