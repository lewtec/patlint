package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"
)

func TestNeededImportKeys(t *testing.T) {
	bindings := []ImportBinding{
		{Local: "fmt", Key: "fmt"},
		{Local: "os", Key: "os"},
		{Local: "*", Key: "import java.util.*;", KeepAlways: true},
		{Local: "_", Key: "image/png"}, // barrel — skip unless KeepAlways
	}
	got := NeededImportKeys("fmt.Println(x)", bindings, nil)
	require.False(t, len(got) != 2 || got[0] != "fmt" || got[1] != "import java.util.*;",
		"got %v", got)

	// dedupe
	bindings = append(bindings, ImportBinding{Local: "fmt", Key: "fmt"})
	got = NeededImportKeys("fmt.X", bindings, nil)
	require.Len(t, got, 2,
		"dedupe: %v", got)

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
	require.False(t, len(edits) != 1 || edits[0].StartByte != 9,
		"edits=%+v", edits)

	// OnlyCandidates limits
	edits = PruneUnusedImportSites("f.go", content, PruneImportOpts{OnlyCandidates: []string{"A"}}, sites, nil)
	require.Empty(t, edits,
		"A is used, want no prune: %+v", edits)

	edits = PruneUnusedImportSites("f.go", content, PruneImportOpts{OnlyCandidates: []string{"B"}}, sites, nil)
	require.Len(t, edits, 1,
		"B only: %+v", edits)

	// KeepAlways
	sites[1].KeepAlways = true
	edits = PruneUnusedImportSites("f.go", content, PruneImportOpts{}, sites, nil)
	require.Empty(t, edits,
		"KeepAlways: %+v", edits)

}

func TestCandidateKeySet(t *testing.T) {
	require.Nil(t, CandidateKeySet(nil),
		"nil")

	m := CandidateKeySet([]string{" a ", "", "b"})
	require.False(t, !m["a"] || !m["b"] || len(m) != 2,
		"%v", m)

}

func TestAppendUnoccupied(t *testing.T) {
	occ := NewSpanSet()
	e1 := project.Edit{Span: ingestutil.Span{0, 2}, NewText: "x"}
	edits := AppendUnoccupied(nil, occ, e1)
	require.Len(t, edits, 1)

	edits = AppendUnoccupied(edits, occ, project.Edit{Span: ingestutil.Span{0, 1}, NewText: "y"})
	require.Len(t, edits, 1,
		"overlap must skip")

	edits = AppendUnoccupied(edits, occ, project.Edit{Span: ingestutil.Span{5, 6}, NewText: "z"})
	require.Len(t, edits, 2)

}
