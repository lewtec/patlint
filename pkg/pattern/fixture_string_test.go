package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

// Ensures fixture pattern strings (not only hand-authored IR) drive the engine.
func TestFixturePatternStrings(t *testing.T) {
	fixtureDir := lewpath.New("..", "..", "testdata", "pattern").String()
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			dir := lewpath.New(fixtureDir, entry.Name()).String()
			fileOp, err := pattern.LoadOp(dir)
			if err != nil {
				t.Fatal(err)
			}
			repl := ""
			if fileOp.Replacement != nil {
				repl = *fileOp.Replacement
			}
			op, err := pattern.OpFromCLI(fileOp.Mode, fileOp.Lang, fileOp.Pattern, repl)
			if err != nil {
				t.Fatalf("OpFromCLI: %v", err)
			}
			tmp := t.TempDir()
			copyDir(t, lewpath.New(dir, "scenario").String(), tmp)
			vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
			if err != nil {
				t.Fatal(err)
			}
			w, err := walker.NewWalker(t.Context(), project.NewSession(tmp).WithEngine(ccgo.Engine{}), vm)
			if err != nil {
				t.Fatal(err)
			}
			switch op.Mode {
			case "grep":
				res, err := w.Run(t.Context(), op, pattern.RunOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if fileOp.ExpectMatchCount != nil && len(res.Matches) != *fileOp.ExpectMatchCount {
					t.Fatalf("matches %d want %d", len(res.Matches), *fileOp.ExpectMatchCount)
				}
			case "rewrite":
				if _, err := w.Apply(t.Context(), op, pattern.RunOptions{}); err != nil {
					t.Fatal(err)
				}
				compareDir(t, lewpath.New(dir, "expected").String(), tmp)
			}
		})
	}
}
