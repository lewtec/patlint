package ingest_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestStageEditsAndValidate(t *testing.T) {
	dir := t.TempDir()
	{
		err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example\n\ngo 1.22\n"), 0o644)
		require.NoError(t, err)
	}

	path := lewpath.New(dir, "main.go").String()
	src := "package main\n\nfunc Hello() {}\n\nfunc main() { Hello() }\n"
	{
		err := os.WriteFile(path, []byte(src), 0o644)
		require.NoError(t, err)
	}

	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	plan, err := w.Rename(t.Context(), dir, "path:./main.go::Hello", "path:./main.go::Hi")
	require.NoError(t, err)
	require.NotEmpty(t, plan.Edits,
		"expected edits")

	ov, err := ingest.StageEdits(t.Context(), dir, nil, plan.Edits)
	require.NoError(t, err)
	{

		err := w.ValidateStaged(t.Context(), dir, ov)
		require.NoError(t, err,
			"validate: %v", err)
	}

	// disk unchanged
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, src, string(got),
		"disk changed: %q", got)

}
