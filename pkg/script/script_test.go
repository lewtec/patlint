package script

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestLoadAndRunPreferAny(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc f(x interface{}) {}\n")
	if err := os.WriteFile(lewpath.New(dir, "x.go").String(), src, 0o644); err != nil {
		t.Fatal(err)
	}
	script := `
(rule go/prefer-any
  warning
  "Prefer any over interface{} (Go 1.18+)"
  (under (lang go)
    (rewrite
      (token "interface{}")
      "any")))
`
	prog, err := Load("test.rft", script)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Actions) != 1 {
		t.Fatalf("actions %d", len(prog.Actions))
	}
	if prog.Actions[0].Report == nil || prog.Actions[0].Report.ID != "go/prefer-any" {
		t.Fatalf("%+v", prog.Actions[0].Report)
	}
	if prog.Actions[0].Emit == nil {
		t.Fatal("want emit")
	}

	res, err := Run(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), prog, Options{Paths: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) < 1 {
		t.Fatalf("want findings, got %d", len(res.Findings))
	}
	if !res.Findings[0].Fixable {
		t.Fatal("want fixable")
	}
	if len(res.ApplyEdits) < 1 {
		t.Fatal("want apply edits")
	}
}

func TestLoadDefAndCall(t *testing.T) {
	script := `
(def os-err-to-errors-is (old-fn err-ref)
  (under (lang go)
    (rewrite
      (seq (ref old-fn) "(" (token "err") ")")
      (seq (ref "go:errors::Is") "(" "err" ", " (ref err-ref) ")"))))

(rule go/os-isexist-err
  warning
  "See docs"
  (os-err-to-errors-is "go:os::IsExist" "go:io/fs::ErrExist"))
`
	prog, err := Load("t.rft", script)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Actions) != 1 {
		t.Fatalf("actions %#v", prog.Actions)
	}
	if prog.Actions[0].Lang != "go" {
		t.Fatalf("lang %q", prog.Actions[0].Lang)
	}
	if prog.Actions[0].Emit == nil {
		t.Fatal("emit")
	}
}

func TestNestedRuleRejectedClearly(t *testing.T) {
	// Nested (rule …) must not become a silent match node or opaque "unknown head".
	cases := []string{
		`(under (lang go) (rule go/x warning "m" (token "y")))`,
		`(def wrap () (rule go/x warning "m" (token "y")))
(wrap)`,
	}
	for _, src := range cases {
		_, err := Load("bad.rft", src)
		if err == nil {
			t.Fatalf("want error for nested rule, src=%q", src)
		}
		if !strings.Contains(err.Error(), "top level") {
			t.Fatalf("want clear top-level message, got %v", err)
		}
		if strings.Contains(err.Error(), `unknown head "rule"`) {
			t.Fatalf("still opaque unknown head: %v", err)
		}
	}
}

func TestTopLevelRewriteNoReport(t *testing.T) {
	// rewrite without rule: edits only, no findings
	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) {}\n")
	if err := os.WriteFile(lewpath.New(dir, "x.go").String(), src, 0o644); err != nil {
		t.Fatal(err)
	}
	script := `
(rewrite
  (under (lang go) (token "interface{}"))
  "any")
`
	prog, err := Load("t.rft", script)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), prog, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("want no lint findings, got %d", len(res.Findings))
	}
	if len(res.ApplyEdits) < 1 {
		t.Fatal("want edits from bare rewrite")
	}
	_ = strings.Contains
}
