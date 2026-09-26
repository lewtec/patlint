package html_test

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/ecma/js"
	_ "github.com/lewtec/patlint/pkg/ingest/html"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/walker"
)

func parseHTML(t *testing.T, src string) (*sitter.Node, []byte, func()) {
	t.Helper()
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, []byte(src), "page.html", "html")
	if err != nil {
		t.Fatal(err)
	}
	return pf.Root, pf.Source, pf.Close
}

func TestHTMLAbstract_ElementsAndScript(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	src := `<!DOCTYPE html>
<html>
<body>
  <div class="card">
    <span>hello</span>
  </div>
  <script>
    function maxAb(a, b) {
      if (a > b) { return a; }
      return b;
    }
  </script>
</body>
</html>
`
	root, source, cleanup := parseHTML(t, src)
	defer cleanup()

	if _, ok := vm.HostLanguage("page.html"); !ok {
		t.Fatal("html language not registered")
	}

	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	units, err := w.AbstractFile(t.Context(), root, source, "page.html", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 2 {
		t.Fatalf("want markup + script units, got %d %#v", len(units), units)
	}

	var sawDiv, sawScriptFunc bool
	for _, u := range units {
		terms := ingest.FormatTerms(u.Terms)
		if u.Name == "div" || strings.Contains(terms, "div") {
			sawDiv = true
		}
		if strings.Contains(terms, "%r1") && strings.Contains(terms, "return") {
			sawScriptFunc = true
		}
		if int(u.Span.EndByte) > len(source) {
			t.Fatalf("span past EOF: %v", u.Span)
		}
	}
	if !sawDiv {
		names := make([]string, len(units))
		for i, u := range units {
			names[i] = u.Name + ":" + ingest.FormatTerms(u.Terms)[:min(40, len(ingest.FormatTerms(u.Terms)))]
		}
		t.Fatalf("expected a div element unit among %v", names)
	}
	if !sawScriptFunc {
		t.Fatalf("expected script function with holes among units: %#v", units)
	}
}

func TestHTMLAbstract_LangTSScript(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	src := `<script lang="ts">
function id(x: number) { return x; }
</script>
`
	root, source, cleanup := parseHTML(t, src)
	defer cleanup()
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	units, err := w.AbstractFile(t.Context(), root, source, "x.html", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 1 {
		t.Fatalf("want script unit from lang=ts, got %d", len(units))
	}
	found := false
	for _, u := range units {
		got := ingest.FormatTerms(u.Terms)
		if strings.Contains(got, "return") && strings.Contains(got, "%r1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ts function with holes: %#v", units)
	}
}
