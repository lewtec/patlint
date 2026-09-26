package js_test

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/ecma/js"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/walker"
)

func parseJS(t *testing.T, src, filename string) (*sitter.Node, []byte, func()) {
	t.Helper()
	lang := "javascript"
	switch {
	case strings.HasSuffix(filename, ".tsx"):
		lang = "tsx"
	case strings.HasSuffix(filename, ".ts"):
		lang = "typescript"
	}
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, []byte(src), filename, lang)
	if err != nil {
		t.Fatal(err)
	}
	return pf.Root, pf.Source, pf.Close
}

func TestJSAbstractHolesAndArrow(t *testing.T) {
	src := `
function maxAb(a, b) {
  if (a > b) { return a; }
  return b;
}
const minXy = (x, y) => {
  if (x > y) { return y; }
  return x;
};
`
	root, source, cleanup := parseJS(t, src, "x.js")
	defer cleanup()
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	units, err := w.AbstractFile(t.Context(), root, source, "x.js", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 2 {
		t.Fatalf("units=%d %#v", len(units), units)
	}
	var maxU, arrowU *ingest.AbstractUnit
	for i := range units {
		u := &units[i]
		got := ingest.FormatTerms(u.Terms)
		if !strings.Contains(got, "%r1") {
			t.Fatalf("%s missing holes: %s", u.Name, got)
		}
		if u.Name == "maxAb" {
			maxU = u
		}
		// arrow may be anonymous
		if strings.Contains(got, "return") && u.Name != "maxAb" {
			arrowU = u
		}
	}
	if maxU == nil || arrowU == nil {
		t.Fatalf("want maxAb + arrow, got %#v", units)
	}
}

func TestTSAbstractHoles(t *testing.T) {
	src := `
function maxAb(a: number, b: number): number {
  if (a > b) { return a; }
  return b;
}
`
	root, source, cleanup := parseJS(t, src, "x.ts")
	defer cleanup()
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	units, err := w.AbstractFile(t.Context(), root, source, "x.ts", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 1 {
		t.Fatal("no units")
	}
	got := ingest.FormatTerms(units[0].Terms)
	if !strings.Contains(got, "%r1") || !strings.Contains(got, "%r2") {
		t.Fatalf("terms=%s", got)
	}
}
