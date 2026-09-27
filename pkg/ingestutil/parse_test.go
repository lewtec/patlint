package ingestutil

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/sitter/treesitter"
	"github.com/stretchr/testify/require"
)

func TestParseSourceFile(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	err := os.WriteFile(path, []byte("package p\n"), 0o644)
	require.NoError(t, err)
	pf, err := ParseSourceFile(t.Context(), treesitter.Engine{}, path, "go")
	require.NoError(t, err)
	defer pf.Close()
	require.Equal(t, "package p\n", string(pf.Source))
	require.NotNil(t, pf.Root)
	pf.Close() // double-close must be safe
}

func TestParseSourceFileEmptyLanguage(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	err := os.WriteFile(path, []byte("package p\n"), 0o644)
	require.NoError(t, err)
	_, err = ParseSourceFile(t.Context(), treesitter.Engine{}, path, "")
	require.Error(t, err)
}

func TestParseSourceFileUnknownLanguage(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.unknownlang").String()
	err := os.WriteFile(path, []byte("x"), 0o644)
	require.NoError(t, err)
	_, err = ParseSourceFile(t.Context(), treesitter.Engine{}, path, "not-a-grammar")
	require.Error(t, err)
}

func TestParseSource(t *testing.T) {
	content := []byte("package p\n")
	pf, err := ParseSource(t.Context(), treesitter.Engine{}, content, "x.go", "go")
	require.NoError(t, err)
	defer pf.Close()
	require.NotNil(t, pf.Root)
	require.Equal(t, string(content), string(pf.Source))
}
