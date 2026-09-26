package script_test

import (
	"os"
	"path/filepath"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ignore"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/report"
	"github.com/lewtec/patlint/pkg/script"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

func writeTestTOML(t *testing.T, dir string, meta script.CaseMeta) {
	t.Helper()
	data, err := toml.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(lewpath.New(dir, script.TestFileName).String(), data, 0o644))
}

func TestAssertFindings_partialMatch(t *testing.T) {
	t.Parallel()
	line := 3
	want := []script.WantFinding{{
		ID:   "imports/unused-named",
		Path: "main.go",
		Line: &line,
	}}
	actual := []report.Finding{
		{RuleID: "imports/unused-named", File: "main.go", Line: 3, Column: 1},
		{RuleID: "other/rule", File: "main.go", Line: 1},
	}
	require.Empty(t, script.AssertFindings(want, actual))
}

func TestAssertFindings_missing(t *testing.T) {
	t.Parallel()
	want := []script.WantFinding{{ID: "imports/unused-named", Path: "main.go"}}
	actual := []report.Finding{{RuleID: "other", File: "main.go"}}
	require.Len(t, script.AssertFindings(want, actual), 1)
}

func TestAssertFindings_extraActualIgnored(t *testing.T) {
	t.Parallel()
	want := []script.WantFinding{{ID: "r1"}}
	actual := []report.Finding{
		{RuleID: "r1", File: "a.go", Line: 1},
		{RuleID: "r1", File: "b.go", Line: 2},
	}
	require.Empty(t, script.AssertFindings(want, actual))
}

func TestAssertFindings_twoRowsNeedTwoHits(t *testing.T) {
	t.Parallel()
	want := []script.WantFinding{
		{ID: "r1", Path: "a.go"},
		{ID: "r1", Path: "b.go"},
	}
	actual := []report.Finding{{RuleID: "r1", File: "a.go"}}
	require.Len(t, script.AssertFindings(want, actual), 1)
}

func TestWalkCasesRecursive_skipsGitignored(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCase := func(dir string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(lewpath.New(dir, "scenario").String(), 0o755))
		writeTestTOML(t, dir, script.CaseMeta{Kind: script.KindRun})
	}
	require.NoError(t, os.WriteFile(lewpath.New(root, ".gitignore").String(), []byte("hidden/\n"), 0o644))
	writeCase(lewpath.New(root, "keep", "nested").String())
	writeCase(lewpath.New(root, "hidden", "skipme").String())

	var names []string
	err := script.WalkCasesRecursive(t.Context(), root, nil, func(c script.Case) error {
		names = append(names, c.Name)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"nested"}, names)
}

func TestLoadCase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(lewpath.New(dir, "scenario").String(), 0o755))
	writeTestTOML(t, dir, script.CaseMeta{
		Kind:        script.KindRun,
		Description: "example",
		Finding: []script.WantFinding{
			{ID: "imports/unused-named", Path: "main.go"},
		},
	})
	c, err := script.LoadCase(dir)
	require.NoError(t, err)
	require.Equal(t, "example", c.Meta.Description)
	require.Len(t, c.Meta.Finding, 1)
	require.Equal(t, "imports/unused-named", c.Meta.Finding[0].ID)
}

func TestDiffTrees(t *testing.T) {
	t.Parallel()
	a := t.TempDir()
	b := t.TempDir()
	require.NoError(t, os.WriteFile(lewpath.New(a, "f.go").String(), []byte("ok\n"), 0o644))
	require.NoError(t, os.WriteFile(lewpath.New(b, "f.go").String(), []byte("ok\n"), 0o644))
	require.Empty(t, script.DiffTrees(t.Context(), a, b))
	require.NoError(t, os.WriteFile(lewpath.New(b, "f.go").String(), []byte("nope\n"), 0o644))
	fails := script.DiffTrees(t.Context(), a, b)
	require.Len(t, fails, 1)
	require.Contains(t, fails[0], "ok")
	require.Contains(t, fails[0], "nope")
}

func TestRunCase_deadImports(t *testing.T) {
	root := t.TempDir()
	caseDir := lewpath.New(root, script.PackSubdir, script.TestdataSubdir, "unused_fmt").String()
	scenario := lewpath.New(caseDir, "scenario").String()
	expected := lewpath.New(caseDir, "expected").String()
	require.NoError(t, os.MkdirAll(scenario, 0o755))
	src := "package main\n\nimport \"fmt\"\n\nfunc main() {}\n"
	require.NoError(t, os.WriteFile(lewpath.New(scenario, "main.go").String(), []byte(src), 0o644))

	gold := t.TempDir()
	require.NoError(t, os.WriteFile(lewpath.New(gold, "main.go").String(), []byte(src), 0o644))
	prog, err := script.EnsureDeadImports(&script.Program{Path: "<builtin>"})
	require.NoError(t, err)
	fixRes, err := script.Run(t.Context(), project.NewSession(gold).WithEngine(ccgo.Engine{}), prog, script.Options{Paths: []string{"."}})
	require.NoError(t, err)
	require.NotEmpty(t, fixRes.ApplyEdits)
	require.NoError(t, project.ApplyEdits(t.Context(), gold, fixRes.ApplyEdits))
	got, err := os.ReadFile(lewpath.New(gold, "main.go").String())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(expected, 0o755))
	require.NoError(t, os.WriteFile(lewpath.New(expected, "main.go").String(), got, 0o644))

	writeTestTOML(t, caseDir, script.CaseMeta{
		Kind: script.KindRun,
		Finding: []script.WantFinding{
			{ID: "imports/unused-named", Path: "main.go"},
		},
	})

	c, err := script.LoadCase(caseDir)
	require.NoError(t, err)
	res, err := script.RunCase(t.Context(), c, script.RunCaseOptions{PackRoot: root})
	require.NoError(t, err)
	require.Empty(t, res.Failures)
}

func TestWalkCasesRecursive_extractWithRepoIgnore(t *testing.T) {
	repo, err := filepath.Abs(lewpath.New("..", "..").String())
	require.NoError(t, err)
	eng, err := ignore.Collect(t.Context(), repo)
	require.NoError(t, err)
	n := 0
	err = script.WalkCasesRecursive(t.Context(), lewpath.New(repo, "testdata", "extract").String(), eng, func(c script.Case) error {
		n++
		return nil
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 10)
}
