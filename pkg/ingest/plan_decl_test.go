package ingest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyDeclWrap(t *testing.T) {
	got := applyDeclarationWrap("type", "Config struct {\n\t\tName string\n\t}", "\t")
	want := "type Config struct {\n\tName string\n}"
	require.Equal(t, want, got,
		"got %q want %q", got, want)
	require.Equal(t, "var ErrNoGoVersions   = ErrNoVersions", applyDeclarationWrap("var", "ErrNoGoVersions   = ErrNoVersions", "\t"),
		"var wrap")

}

func TestFirstIdentToken(t *testing.T) {
	src := []byte("type ( A; B )")
	{
		got := firstIdentifierToken(src, 0, uint32(len(src)))
		require.Equal(t, "type", got,
			"got %q", got)
	}

	got := firstIdentifierToken([]byte("  const (\n\tC = iota\n)"), 0, 20)
	require.Equal(t, "const", got,
		"got %q", got)

}
