package ingest

import "testing"

func TestFindAllOccurrencesInStringsUsesWalker(t *testing.T) {
	// Second literal must not sit after // — ForEachStringLiteral skips line comments.
	content := []byte("import \"old/pkg\"; // old/pkg outside\nvar s = \"old/pkg\"\n")
	edits := FindAllOccurrencesInStrings("f.go", content, "old/pkg", "new/pkg")
	if len(edits) != 2 {
		t.Fatalf("want 2 string matches, got %d: %#v", len(edits), edits)
	}
	for _, e := range edits {
		if e.NewText != "new/pkg" {
			t.Fatalf("bad edit: %#v", e)
		}
	}
}
