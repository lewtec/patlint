package ingest_test

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/stretchr/testify/require"
)

func TestParseReference_ExplicitProvider(t *testing.T) {
	ref := ingest.ParseReference("path:./cmd/rft/doc.go::newDocCmd")
	require.Equal(t, ingest.Reference{Provider: "path", Path: "./cmd/rft/doc.go", Name: "newDocCmd"}, ref)
	require.Equal(t, "path:./cmd/rft/doc.go::newDocCmd", ref.String())

}

func TestParseReference_UppercaseProviderCanonicalized(t *testing.T) {
	ref := ingest.ParseReference("Path:./cmd/rft/doc.go::newDocCmd")
	require.Equal(t, ingest.Reference{Provider: "path", Path: "./cmd/rft/doc.go", Name: "newDocCmd"}, ref)
	require.Equal(t, "path:./cmd/rft/doc.go::newDocCmd", ref.String())

}

func TestParseReference_ShorthandPathSymbol(t *testing.T) {
	ref := ingest.ParseReference("cmd/rft/doc.go::newDocCmd")
	require.Equal(t, ingest.Reference{Provider: "path", Path: "./cmd/rft/doc.go", Name: "newDocCmd"}, ref)
	require.Equal(t, "path:./cmd/rft/doc.go::newDocCmd", ref.String())

}

func TestParseReference_NonPathProvider(t *testing.T) {
	ref := ingest.ParseReference("go:fmt::Println")
	require.Equal(t, ingest.Reference{Provider: "go", Path: "fmt", Name: "Println"}, ref)
	require.Equal(t, "go:fmt::Println", ref.String())

}
