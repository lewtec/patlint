package script_test

import (
	"os"
	"path/filepath"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/script"
)

func TestListPackScripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := lewpath.New(root, rel).String()
		{
			err := os.MkdirAll(filepath.Dir(p), 0o755)
			require.NoError(t, err)
		}

		err := os.WriteFile(p, []byte(body), 0o644)
		require.NoError(t, err)

	}
	write("z.rft", ";; z\n")
	write("a.rft", ";; a\n")
	write(lewpath.New(script.PackSubdir, "b.rft").String(), ";; b\n")
	write(lewpath.New(script.PackSubdir, "nested", "skip.rft").String(), ";; no recurse\n")
	write("not-rft.txt", "x\n")

	got, err := script.ListPackScripts(root)
	require.NoError(t, err)
	require.Len(t, got, 3,
		"got %v want 3 scripts (no nested)", got)

	for _, p := range got {
		require.NotEqual(t, filepath.Base(filepath.Dir(p)), "nested",
			"recursive leak: %s", p)

	}
	got2, err := script.ListPackScripts(root)
	require.NoError(t, err)

	for i := range got {
		require.Equal(t, got2[i], got[i],
			"nondeterministic: %v vs %v", got, got2)

	}
}

func TestExpandScriptArgs_fileAndDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	file := lewpath.New(root, "one.rft").String()
	{
		err := os.WriteFile(file, []byte(";;\n"), 0o644)
		require.NoError(t, err)
	}

	pack := lewpath.New(root, "pack").String()
	{
		err := os.MkdirAll(lewpath.New(pack, script.PackSubdir).String(), 0o755)
		require.NoError(t, err)
	}

	packScript := lewpath.New(pack, script.PackSubdir, "two.rft").String()
	{
		err := os.WriteFile(packScript, []byte(";;\n"), 0o644)
		require.NoError(t, err)
	}

	got, err := script.ExpandScriptArgs([]string{file, pack})
	require.NoError(t, err)
	require.Len(t, got, 2,
		"got %v", got)

}

func TestExpandScriptArgs_emptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := script.ExpandScriptArgs([]string{dir})
	require.Error(t, err,
		"expected error for empty pack dir")

}

func TestEnsureDeadImports(t *testing.T) {
	t.Parallel()
	p := &script.Program{Path: "t.rft"}
	p, err := script.EnsureDeadImports(p)
	require.NoError(t, err)
	require.False(t, len(p.Actions) < 1 || p.Actions[0].Builtin != script.BuiltinDeadImports,
		"actions=%+v", p.Actions)

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
		require.True(t, ids[id],
			"missing always-on %s; have %v", id, ids)

	}
	n := len(p.Actions)
	p2, err := script.EnsureDeadImports(p)
	require.NoError(t, err)
	require.Len(t, p2.Actions, n,
		"not idempotent: %d → %d", n, len(p2.Actions))

}

func TestMergePrograms(t *testing.T) {
	t.Parallel()
	a := &script.Program{Actions: []script.Action{{Source: "a"}}}
	b := &script.Program{Actions: []script.Action{{Source: "b"}, {Source: "c"}}}
	m := script.MergePrograms("a+b", []*script.Program{a, b})
	require.False(t, len(m.Actions) != 3 || m.Path != "a+b",
		"%+v", m)

}
