package ingest

import (
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

import "testing"

func TestNeededImportKeys(t *testing.T) {
	bindings := []ImportBinding{
		{Local: "fmt", Key: "fmt"},
		{Local: "os", Key: "os"},
		{Local: "*", Key: "import java.util.*;", KeepAlways: true},
		{Local: "_", Key: "image/png"}, // barrel — skip unless KeepAlways
	}
	got := NeededImportKeys("fmt.Println(x)", bindings, nil)
	if len(got) != 2 || got[0] != "fmt" || got[1] != "import java.util.*;" {
		t.Fatalf("got %v", got)
	}
	// dedupe
	bindings = append(bindings, ImportBinding{Local: "fmt", Key: "fmt"})
	got = NeededImportKeys("fmt.X", bindings, nil)
	if len(got) != 2 {
		t.Fatalf("dedupe: %v", got)
	}
}

func TestPruneUnusedImportSites(t *testing.T) {
	content := []byte("import A\nimport B\n\nuse A\n")
	// mask nothing; B unused
	sites := []ImportSite{
		{Key: "import A", Locals: []string{"A"}, Span: ingestutil.Span{0, 8}}, // "import A" without \n — rough
		{Key: "import B", Locals: []string{"B"}, Span: ingestutil.Span{9, 17}},
	}
	// Fix spans to match content
	sites[0].Span = ingestutil.Span{0, 8} // import A
	// content: "import A\nimport B\n\nuse A\n"
	// positions: 0-8 import A, 8=\n, 9-17 import B
	sites[0] = ImportSite{Key: "A", Locals: []string{"A"}, Span: ingestutil.Span{0, 9}} // include newline
	sites[1] = ImportSite{Key: "B", Locals: []string{"B"}, Span: ingestutil.Span{9, 18}}

	edits := PruneUnusedImportSites("f.go", content, PruneImportOpts{}, sites, nil)
	if len(edits) != 1 || edits[0].StartByte != 9 {
		t.Fatalf("edits=%+v", edits)
	}

	// OnlyCandidates limits
	edits = PruneUnusedImportSites("f.go", content, PruneImportOpts{OnlyCandidates: []string{"A"}}, sites, nil)
	if len(edits) != 0 {
		t.Fatalf("A is used, want no prune: %+v", edits)
	}
	edits = PruneUnusedImportSites("f.go", content, PruneImportOpts{OnlyCandidates: []string{"B"}}, sites, nil)
	if len(edits) != 1 {
		t.Fatalf("B only: %+v", edits)
	}

	// KeepAlways
	sites[1].KeepAlways = true
	edits = PruneUnusedImportSites("f.go", content, PruneImportOpts{}, sites, nil)
	if len(edits) != 0 {
		t.Fatalf("KeepAlways: %+v", edits)
	}
}

func TestCandidateKeySet(t *testing.T) {
	if CandidateKeySet(nil) != nil {
		t.Fatal("nil")
	}
	m := CandidateKeySet([]string{" a ", "", "b"})
	if !m["a"] || !m["b"] || len(m) != 2 {
		t.Fatalf("%v", m)
	}
}

func TestAppendUnoccupied(t *testing.T) {
	occ := NewSpanSet()
	e1 := project.Edit{Span: ingestutil.Span{0, 2}, NewText: "x"}
	edits := AppendUnoccupied(nil, occ, e1)
	if len(edits) != 1 {
		t.Fatal(edits)
	}
	edits = AppendUnoccupied(edits, occ, project.Edit{Span: ingestutil.Span{0, 1}, NewText: "y"})
	if len(edits) != 1 {
		t.Fatal("overlap must skip")
	}
	edits = AppendUnoccupied(edits, occ, project.Edit{Span: ingestutil.Span{5, 6}, NewText: "z"})
	if len(edits) != 2 {
		t.Fatal(edits)
	}
}
