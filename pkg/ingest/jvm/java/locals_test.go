package java_test

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/java"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/walker"
)

func TestJavaAbstractHoles(t *testing.T) {
	src := `class C {
  int max(int a, int b) {
    if (a > b) { return a; }
    return b;
  }
  int max2(int x, int y) {
    if (x > y) { return x; }
    return y;
  }
}
`
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, []byte(src), "C.java", "java")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	units, err := w.AbstractFile(t.Context(), pf.Root, pf.Source, "C.java", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 2 {
		t.Fatalf("units=%d", len(units))
	}
	for _, u := range units {
		got := ingest.FormatTerms(u.Terms)
		if !strings.Contains(got, "%r1") || !strings.Contains(got, "%r2") {
			t.Fatalf("%s: %s", u.Name, got)
		}
	}
	var twins []string
	for _, u := range units {
		if u.Name == "max" || u.Name == "max2" {
			twins = append(twins, ingest.FormatTerms(u.Terms))
		}
	}
	if len(twins) < 2 {
		t.Fatalf("method units=%v", units)
	}
	if twins[0] != twins[1] {
		t.Fatalf("rename twins differ:\n%s\n%s", twins[0], twins[1])
	}
}
