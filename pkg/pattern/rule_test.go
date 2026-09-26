package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/store"
)

func TestRuleFromStrings_Take(t *testing.T) {
	rule, err := pattern.RuleFromStrings(`(take "c" (capture c (ref "go:context::Background")))`, `(ref "go:testing::T")`)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Take != "c" {
		t.Fatalf("Take=%q", rule.Take)
	}
	if rule.Emit == nil {
		t.Fatal("empty emit")
	}
	if rule.Matcher == nil {
		t.Fatal("RuleFromStrings should keep compiled Matcher")
	}
}

func TestRuleFromOp_KeepsMatcher(t *testing.T) {
	op, err := pattern.OpFromCLI("rewrite", "go", `(token "interface{}")`, `any`)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := pattern.RuleFromOp(op)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Matcher == nil {
		t.Fatal("RuleFromOp should attach Matcher")
	}
}

func TestRefLeafRule_ExpandFile(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import "fmt"

func f() {
	fmt.Println("x")
}
`)
	path := lewpath.New(dir, "p.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	sess := project.NewSession(".").WithEngine(ccgo.Engine{})
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	fe, err := ingest.CollectExtracts(t.Context(), ingest.ExtractSource{
		Kind:    ingest.ExtractHop,
		Root:    dir,
		Paths:   []string{path},
		Session: sess,
		Policy:  vm,
	})
	if err != nil {
		t.Fatal(err)
	}
	st := store.New()
	ingest.Ingest(st, fe, vm)
	result, err := ingest.EvalStore(t.Context(), dir, st, vm)
	if err != nil {
		t.Fatal(err)
	}

	pf, err := ingestutil.ParseSourceFile(t.Context(), ccgo.Engine{}, path, "go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()

	target := ""
	for _, u := range result.Uses {
		if u.Target != "" {
			target = u.Target
			break
		}
	}
	if target == "" {
		t.Fatalf("no uses in result; atoms=%d uses=%d", len(result.Atoms), len(result.Uses))
	}

	rule, err := pattern.RefLeafRule(target, "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if !rule.NeedsLinks() {
		t.Fatal("RefLeafRule should need links")
	}

	matches, edits, err := rule.ExpandFile(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), dir, "p.go", src, pf.Root, result)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected matches for target %q", target)
	}
	if len(edits) == 0 {
		t.Fatal("expected edits")
	}
	for _, e := range edits {
		if e.File != "p.go" {
			t.Fatalf("edit file=%q", e.File)
		}
		if e.NewText != "Renamed" {
			t.Fatalf("NewText=%q", e.NewText)
		}
		if e.StartByte >= e.EndByte {
			t.Fatalf("empty span: %+v", e)
		}
	}
}

func TestRuleFromOp_Rewrite(t *testing.T) {
	op, err := pattern.OpFromCLI("rewrite", "go", `(token "interface{}")`, `any`)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := pattern.RuleFromOp(op)
	if err != nil {
		t.Fatal(err)
	}
	if err := rule.Valid(); err != nil {
		t.Fatal(err)
	}
}
