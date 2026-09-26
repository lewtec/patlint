package pattern_test

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestEmitRefs(t *testing.T) {
	emit, err := pattern.ParseEmit(`(ref "go:context::Background")`)
	if err != nil {
		t.Fatal(err)
	}
	refs := pattern.EmitRefs(emit)
	if len(refs) != 1 || !strings.Contains(refs[0], "context") {
		t.Fatalf("refs=%v", refs)
	}
}

func TestWithImportHygiene_AddsFmt(t *testing.T) {
	dir := t.TempDir()
	// File has no fmt import; rewrite will emit fmt.Errorf via (ref …).
	src := []byte("package p\n\nfunc f() error {\n\treturn nil\n}\n")
	path := lewpath.New(dir, "p.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	op, err := pattern.OpFromCLI("rewrite", "go",
		`(seq (token "return") (token "nil"))`,
		`(seq "return " (ref "go:fmt::Errorf") "(\"x\")")`,
	)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	res, err := w.Run(t.Context(), op, pattern.RunOptions{Paths: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Edits) == 0 {
		t.Fatal("expected edits")
	}
	if err := project.ApplyEdits(t.Context(), dir, res.Edits); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "fmt.Errorf") {
		t.Fatalf("missing fmt.Errorf:\n%s", text)
	}
	if !strings.Contains(text, `"fmt"`) {
		t.Fatalf("missing import fmt:\n%s", text)
	}
}

func TestImportNeedsForRule(t *testing.T) {
	rule, err := pattern.RuleFromStrings(`(token "x")`, `(ref "go:net/http::Get")`)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	needs := pattern.ImportNeedsForRule(vm, "go", rule)
	if len(needs) != 1 || needs[0].ImportPath != "net/http" {
		t.Fatalf("needs=%+v", needs)
	}
	if py := pattern.ImportNeedsForRule(vm, "python", rule); len(py) != 0 {
		t.Fatalf("python must not import go refs: %+v", py)
	}
}

func TestWithImportHygiene_PrunesNamedAfterSiteRewrite(t *testing.T) {
	dir := t.TempDir()
	// context only used at the rewritten site; after rewrite it is unused and pruned.
	src := []byte("package p\n\nimport (\n\t\"context\"\n\t\"fmt\"\n)\n\nfunc f() {\n\t_ = context.Background()\n\tfmt.Println()\n}\n")
	path := lewpath.New(dir, "p.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	op, err := pattern.OpFromCLI("rewrite", "go",
		`(seq (token "context") "." (token "Background") "(" ")")`,
		`nil`,
	)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	res, err := w.Run(t.Context(), op, pattern.RunOptions{Paths: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	if err := project.ApplyEdits(t.Context(), dir, res.Edits); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, `"context"`) {
		t.Fatalf("expected context import pruned:\n%s", text)
	}
	if !strings.Contains(text, `"fmt"`) {
		t.Fatalf("fmt must remain:\n%s", text)
	}
	if !strings.Contains(text, "nil") {
		t.Fatalf("rewrite missing:\n%s", text)
	}
}
