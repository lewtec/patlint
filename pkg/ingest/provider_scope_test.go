package ingest_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/python"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestMaterializeProviderScope_SingleFileNotWholeDir(t *testing.T) {
	dir := t.TempDir()
	// Fake "stdlib" with many peers; only one should be hopped.
	for _, name := range []string{"os.py", "sys.py", "json.py", "target.py"} {
		if err := os.WriteFile(lewpath.New(dir, name).String(), []byte("x = 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	res, err := w.LoadProviderScope(t.Context(), ingest.ProviderScopeTarget{
		Dir:  dir,
		File: "target.py",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 1 {
		t.Fatalf("files=%d want 1 (got %v)", len(res.Files), res.Files)
	}
	// Basename relative to Dir
	got := res.Files[0].Path
	if got != "target.py" && got != "./target.py" {
		t.Fatalf("path=%q", got)
	}
}
