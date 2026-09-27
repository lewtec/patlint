package ingest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindAllOccurrencesInStringsUsesWalker(t *testing.T) {
	// Second literal must not sit after // — ForEachStringLiteral skips line comments.
	content := []byte("import \"old/pkg\"; // old/pkg outside\nvar s = \"old/pkg\"\n")
	edits := FindAllOccurrencesInStrings("f.go", content, "old/pkg", "new/pkg")
	require.Len(t, edits, 2,
		"want 2 string matches, got %d: %#v", len(edits), edits)

	for _, e := range edits {
		require.Equal(t, "new/pkg", e.NewText,
			"bad edit: %#v", e)

	}
}
