package pattern

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestRewriteTakeOnly(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import (
	"context"
	"testing"
)

func TestFoo(t *testing.T) {
	_ = context.Background()
	_ = context.Background()
}

func Helper() {
	_ = context.Background()
}
`)
	path := lewpath.New(dir, "x_test.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	// Without *: only first Background in TestFoo.
	op, err := OpFromCLI("rewrite", "go",
		`(take "c" (seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (capture c (ref "go:context::Background")) (* any) "}"))`,
		`t.Context`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if op.Take != "c" {
		t.Fatalf("Take=%q want c", op.Take)
	}
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	res, err := applyOp(t.Context(), sess, vm, op, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Edits) != 1 {
		t.Fatalf("edits=%d want 1 (first only); edits=%v", len(res.Edits), res.Edits)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Count(got, "t.Context()") != 1 {
		t.Fatalf("t.Context count=%d want 1\n%s", strings.Count(got, "t.Context()"), got)
	}
	if strings.Count(got, "context.Background") != 2 {
		t.Fatalf("context.Background count=%d want 2 (second Test + Helper)\n%s",
			strings.Count(got, "context.Background"), got)
	}
}

func TestRewriteTakeMultiStar(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import (
	"context"
	"testing"
)

func TestFoo(t *testing.T) {
	_ = context.Background()
	_ = context.Background()
}

func Helper() {
	_ = context.Background()
}
`)
	path := lewpath.New(dir, "x_test.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	// With *: every Background inside each Test* function.
	op, err := OpFromCLI("rewrite", "go",
		`(take "c" (seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (* (capture c (ref "go:context::Background"))) (* any) "}"))`,
		`t.Context`,
	)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	res, err := applyOp(t.Context(), sess, vm, op, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Edits) != 2 {
		t.Fatalf("edits=%d want 2; edits=%v", len(res.Edits), res.Edits)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Count(got, "t.Context()") != 2 {
		t.Fatalf("t.Context count=%d want 2\n%s", strings.Count(got, "t.Context()"), got)
	}
	if strings.Count(got, "context.Background") != 1 {
		t.Fatalf("context.Background count=%d want 1 (Helper only)\n%s",
			strings.Count(got, "context.Background"), got)
	}
}

func TestParseMultiStar(t *testing.T) {
	n, err := ParsePattern(`(* (capture c (ref "go:context::Background")))`)
	if err != nil {
		t.Fatal(err)
	}
	if n.Kind != "ref" || !n.Multi || n.MultiPlus || n.As != "c" {
		t.Fatalf("got %+v", n)
	}
	n, err = ParsePattern(`(* (capture c (group (ref "go:context::Background"))))`)
	if err != nil {
		t.Fatal(err)
	}
	if n.Kind != "group" || !n.Multi || n.MultiPlus {
		t.Fatalf("group multi got %+v", n)
	}
	n, err = ParsePattern(`(* (capture c (group (seq (token "t") "." (token "Context")))))`)
	if err != nil {
		t.Fatal(err)
	}
	if n.Kind != "group" || !n.Multi || n.MultiPlus || n.As != "c" {
		t.Fatalf("t.Context multi group %+v", n)
	}
	n, err = ParsePattern(`(+ (capture c (ref "go:context::Background")))`)
	if err != nil {
		t.Fatal(err)
	}
	// (+) expands; accept Rep+ or Seq of first+rest.
	if n.Kind != "seq" && !(n.Multi && n.MultiPlus) {
		t.Fatalf("plus got %+v", n)
	}
}

func TestMultiPlusRequiresOneSite(t *testing.T) {
	dir := t.TempDir()
	// No context.Background — * would still match the Test envelope; + must not.
	src := []byte(`package p

import "testing"

func TestFoo(t *testing.T) {
	_ = 1
}
`)
	path := lewpath.New(dir, "x_test.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	star, err := ParsePattern(`(seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (* (capture c (ref "go:context::Background"))) (* any) "}")`)
	if err != nil {
		t.Fatal(err)
	}
	plus, err := ParsePattern(`(seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (+ (capture c (ref "go:context::Background"))) (* any) "}")`)
	if err != nil {
		t.Fatal(err)
	}
	msStar := mustMatchFile(t, dir, path, "x_test.go", src, star)
	if len(msStar) != 1 {
		t.Fatalf("* matches=%d want 1 (zero multi still matches envelope)", len(msStar))
	}
	msPlus := mustMatchFile(t, dir, path, "x_test.go", src, plus)
	if len(msPlus) != 0 {
		t.Fatalf("+ matches=%d want 0 when no Background", len(msPlus))
	}

	src2 := []byte(`package p

import (
	"context"
	"testing"
)

func TestFoo(t *testing.T) {
	_ = context.Background()
}
`)
	path2 := lewpath.New(dir, "y_test.go").String()
	if err := os.WriteFile(path2, src2, 0o644); err != nil {
		t.Fatal(err)
	}
	msPlus = mustMatchFile(t, dir, path2, "y_test.go", src2, plus)
	if len(msPlus) != 1 {
		t.Fatalf("+ with Background matches=%d want 1", len(msPlus))
	}
}

func TestRewriteMultiGroupTContext(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import "testing"

func TestFoo(t *testing.T) {
	_ = t.Context()
	_ = t.Context()
}

func Helper(t *testing.T) {
	_ = t.Context()
}
`)
	path := lewpath.New(dir, "x_test.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	op, err := OpFromCLI("rewrite", "go",
		`(take "c" (seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (* (capture c (group (seq (token "t") "." (token "Context"))))) (* any) "}"))`,
		`(ref "go:context::Background")`,
	)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	res, err := applyOp(t.Context(), sess, vm, op, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// 2 site leaf rewrites + import "context" ensure for @go:context::Background
	if len(res.Edits) < 2 {
		t.Fatalf("edits=%d want >=2; %v", len(res.Edits), res.Edits)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Count(got, "context.Background()") != 2 {
		t.Fatalf("context.Background count=%d\n%s", strings.Count(got, "context.Background()"), got)
	}
	if strings.Count(got, "t.Context()") != 1 {
		t.Fatalf("Helper should keep one t.Context:\n%s", got)
	}
	if !strings.Contains(got, `"context"`) {
		t.Fatalf("expected import context:\n%s", got)
	}
}

func TestRewriteNestedMultiIteration(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import "testing"

func TestFoo(t *testing.T) {
	_ = t.Context()
	_ = t.Context()
}
`)
	path := lewpath.New(dir, "x_test.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	op, err := OpFromCLI("rewrite", "go",
		`(take "c" (seq (token "func") (regex "Test.*") "(" (token "t") "*" (token "testing") "." (token "T") ")" "{" (* any) (* (capture i (group (seq (capture c (group (seq (token "t") "." (token "Context")))) (* any))))) "}"))`,
		`(ref "go:context::Background")`,
	)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	res, err := applyOp(t.Context(), sess, vm, op, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// 2 site rewrites + import ensure for @go:context::Background
	if len(res.Edits) < 2 {
		t.Fatalf("edits=%d want >=2; %v", len(res.Edits), res.Edits)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Count(got, "context.Background()") != 2 {
		t.Fatalf("context.Background count=%d\n%s", strings.Count(got, "context.Background()"), got)
	}
	if !strings.Contains(got, `"context"`) {
		t.Fatalf("expected import context:\n%s", got)
	}
}
