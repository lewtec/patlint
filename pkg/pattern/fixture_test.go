package pattern_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/google/go-cmp/cmp"
	"github.com/lewtec/patlint/pkg/script"
)

func TestPatternFixtures(t *testing.T) {
	cases, err := script.LoadCases(t.Context(), lewpath.New("..", "..", "testdata", "pattern").String())
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no testdata/pattern cases")
	}
	for _, c := range cases {
		t.Run(c.Rel(lewpath.New("..", "..").String()), func(t *testing.T) {
			res, err := script.RunCase(t.Context(), c, script.RunCaseOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range res.Failures {
				t.Error(f)
			}
		})
	}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		sp := lewpath.New(src, e.Name()).String()
		dp := lewpath.New(dst, e.Name()).String()
		if e.IsDir() {
			if err := os.MkdirAll(dp, 0o755); err != nil {
				t.Fatal(err)
			}
			copyDir(t, sp, dp)
			continue
		}
		in, err := os.Open(sp)
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.Create(dp)
		if err != nil {
			in.Close()
			t.Fatal(err)
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func compareDir(t *testing.T, expectedDir, gotDir string) {
	t.Helper()
	err := filepath.WalkDir(expectedDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(expectedDir, path)
		if err != nil {
			return err
		}
		expContent, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		gotPath := lewpath.New(gotDir, rel).String()
		gotContent, err := os.ReadFile(gotPath)
		if err != nil {
			t.Errorf("missing file %s: %v", rel, err)
			return nil
		}
		if string(expContent) != string(gotContent) {
			t.Errorf("file %s mismatch (-expected +got):\n%s",
				rel, cmp.Diff(string(expContent), string(gotContent)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
