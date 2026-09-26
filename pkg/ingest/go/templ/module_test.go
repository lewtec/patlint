package templ_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	_ "github.com/lewtec/patlint/pkg/ingest/go/templ"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestTemplLanguageForFile(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	lang, ok := vm.HostLanguage("views/code.templ")
	if !ok || lang != "templ" {
		t.Fatalf("got %q ok=%v want templ", lang, ok)
	}
	if got := vm.FamilyForLanguage("templ"); got != "go" {
		t.Fatalf("family=%q want go", got)
	}
}

func TestTemplIngestComponents(t *testing.T) {
	dir := t.TempDir()
	src := `package views

import "fmt"

templ CodeSegments(segments []string) {
	<pre>
		for _, s := range segments {
			@codeSegment(s)
		}
	</pre>
}

templ codeSegment(s string) {
	<span>{ s }</span>
}

func SpanFragmentID(start, end uint32) string {
	return fmt.Sprintf("%d-%d", start, end)
}
`
	if err := os.WriteFile(lewpath.New(dir, "code.templ").String(), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	// Companion go.mod so package paths resolve if needed
	if err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	result, err := w.Load(t.Context(), ingest.SourceProject(dir), ingest.MaterializeOptions{ExpandImports: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Language != "templ" {
		t.Fatalf("files=%#v", result.Files)
	}

	names := map[string]bool{}
	for _, e := range result.Atoms {
		ref := ingest.ParseReference(e.Reference)
		names[ref.Name] = true
	}
	for _, want := range []string{"CodeSegments", "codeSegment", "SpanFragmentID"} {
		if !names[want] {
			t.Fatalf("missing atom %q in %#v", want, names)
		}
	}

	// @codeSegment usage should resolve to the component atom.
	usageTargets := map[string]bool{}
	for _, r := range result.Uses {
		tgt := ingest.ParseReference(r.Target)
		usageTargets[tgt.Name] = true
	}
	if !usageTargets["codeSegment"] {
		var dump []string
		for _, r := range result.Uses {
			dump = append(dump, r.Reference+" -> "+r.Target)
		}
		t.Fatalf("missing use of codeSegment; uses=%v", dump)
	}
	// fmt.Sprintf usage from helper
	if !usageTargets["Sprintf"] {
		var dump []string
		for _, r := range result.Uses {
			dump = append(dump, r.Reference+" -> "+r.Target)
		}
		t.Fatalf("missing use of Sprintf; uses=%v", dump)
	}
}
