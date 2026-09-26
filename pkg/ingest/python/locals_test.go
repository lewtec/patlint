package python_test

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/python"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/walker"
)

func parsePy(t *testing.T, src string) (*sitter.Node, []byte, func()) {
	t.Helper()
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, []byte(src), "x.py", "python")
	if err != nil {
		t.Fatal(err)
	}
	return pf.Root, pf.Source, pf.Close
}

func TestPythonAbstractHoles(t *testing.T) {
	src := `def max_ab(a, b):
    if a > b:
        return a
    return b

def max_xy(x, y):
    if x > y:
        return x
    return y
`
	root, source, cleanup := parsePy(t, src)
	defer cleanup()
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	units, err := w.AbstractFile(t.Context(), root, source, "x.py", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 2 {
		t.Fatalf("units=%d", len(units))
	}
	for _, u := range units {
		got := ingest.FormatTerms(u.Terms)
		if !strings.Contains(got, "%r1") || !strings.Contains(got, "%r2") {
			t.Fatalf("%s terms lack holes: %s", u.Name, got)
		}
		if strings.Contains(got, " a ") || strings.Contains(got, " b ") ||
			strings.Contains(got, " x ") || strings.Contains(got, " y ") {
			t.Fatalf("%s still has surface names: %s", u.Name, got)
		}
	}
	// rename-identical: same abstract terms
	if ingest.FormatTerms(units[0].Terms) != ingest.FormatTerms(units[1].Terms) {
		t.Fatalf("renamed twins should match:\n%s\n%s", ingest.FormatTerms(units[0].Terms), ingest.FormatTerms(units[1].Terms))
	}
}
