package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

// Ensures fixture pattern strings (not only hand-authored IR) drive the engine.
func TestFixturePatternStrings(t *testing.T) {
	fixtureDir := lewpath.New("..", "..", "testdata", "pattern").String()
	entries, err := os.ReadDir(fixtureDir)
	require.NoError(t, err)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			dir := lewpath.New(fixtureDir, entry.Name()).String()
			fileOp, err := pattern.LoadOp(dir)
			require.NoError(t, err)

			repl := ""
			if fileOp.Replacement != nil {
				repl = *fileOp.Replacement
			}
			op, err := pattern.OpFromCLI(fileOp.Mode, fileOp.Lang, fileOp.Pattern, repl)
			require.NoError(t, err)

			tmp := t.TempDir()
			copyDir(t, lewpath.New(dir, "scenario").String(), tmp)
			vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
			require.NoError(t, err)

			w, err := walker.NewWalker(t.Context(), project.NewSession(tmp).WithEngine(treesitter.Engine{}), vm)
			require.NoError(t, err)

			switch op.Mode {
			case "grep":
				res, err := w.Run(t.Context(), op, pattern.RunOptions{})
				require.NoError(t, err)
				if fileOp.ExpectMatchCount != nil {
					require.Len(t, res.Matches, *fileOp.ExpectMatchCount)
				}

			case "rewrite":
				_, err = w.Apply(t.Context(), op, pattern.RunOptions{})
				require.NoError(t, err)

				compareDir(t, lewpath.New(dir, "expected").String(), tmp)
			}
		})
	}
}
