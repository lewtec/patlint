package script_test

import (
	"os"
	"path/filepath"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/script"
)

func TestListPackScripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := lewpath.New(root, rel).String()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("z.rft", ";; z\n")
	write("a.rft", ";; a\n")
	write(lewpath.New(script.PackSubdir, "b.rft").String(), ";; b\n")
	write(lewpath.New(script.PackSubdir, "nested", "skip.rft").String(), ";; no recurse\n")
	write("not-rft.txt", "x\n")

	got, err := script.ListPackScripts(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %v want 3 scripts (no nested)", got)
	}
	for _, p := range got {
		if filepath.Base(filepath.Dir(p)) == "nested" {
			t.Fatalf("recursive leak: %s", p)
		}
	}
	got2, err := script.ListPackScripts(root)
	if err != nil {
		t.Fatal(err)
	}
	for i := range got {
		if got[i] != got2[i] {
			t.Fatalf("nondeterministic: %v vs %v", got, got2)
		}
	}
}

func TestExpandScriptArgs_fileAndDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	file := lewpath.New(root, "one.rft").String()
	if err := os.WriteFile(file, []byte(";;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pack := lewpath.New(root, "pack").String()
	if err := os.MkdirAll(lewpath.New(pack, script.PackSubdir).String(), 0o755); err != nil {
		t.Fatal(err)
	}
	packScript := lewpath.New(pack, script.PackSubdir, "two.rft").String()
	if err := os.WriteFile(packScript, []byte(";;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := script.ExpandScriptArgs([]string{file, pack})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestExpandScriptArgs_emptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := script.ExpandScriptArgs([]string{dir})
	if err == nil {
		t.Fatal("expected error for empty pack dir")
	}
}

func TestEnsureDeadImports(t *testing.T) {
	t.Parallel()
	p := &script.Program{Path: "t.rft"}
	p = script.EnsureDeadImports(p)
	if len(p.Actions) < 1 || p.Actions[0].Builtin != script.BuiltinDeadImports {
		t.Fatalf("actions=%+v", p.Actions)
	}
	ids := map[string]bool{}
	for _, a := range p.Actions {
		if a.Report != nil {
			ids[a.Report.ID] = true
		}
	}
	for _, id := range []string{
		script.DeadImportsRuleID,
		"rft/until-body-scan",
		"rft/callish-widen",
		"rft/lookbehind-open",
	} {
		if !ids[id] {
			t.Fatalf("missing always-on %s; have %v", id, ids)
		}
	}
	n := len(p.Actions)
	p2 := script.EnsureDeadImports(p)
	if len(p2.Actions) != n {
		t.Fatalf("not idempotent: %d → %d", n, len(p2.Actions))
	}
}

func TestMergePrograms(t *testing.T) {
	t.Parallel()
	a := &script.Program{Actions: []script.Action{{Source: "a"}}}
	b := &script.Program{Actions: []script.Action{{Source: "b"}, {Source: "c"}}}
	m := script.MergePrograms("a+b", []*script.Program{a, b})
	if len(m.Actions) != 3 || m.Path != "a+b" {
		t.Fatalf("%+v", m)
	}
}
