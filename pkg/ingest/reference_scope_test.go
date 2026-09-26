package ingest_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
)

func TestResolveReferenceScope_PathDirectory(t *testing.T) {
	tmp := t.TempDir()
	old, err := os.Getwd()
	require.NoError(t, err)

	defer func() { _ = os.Chdir(old) }()
	{
		err := os.Chdir(tmp)
		require.NoError(t, err)
	}
	{

		err := os.MkdirAll("cmd", 0755)
		require.NoError(t, err)
	}

	scope := ingest.ResolveReferenceScope(".", ingest.ParseReference("path:./cmd"))
	require.Equal(t, "cmd", scope.Dir,
		"unexpected dir: %q", scope.Dir)
	require.False(t, scope.Reference.Provider != "path" || scope.Reference.Path != "./",
		"unexpected normalized ref: %+v", scope.Reference)

}

func TestNormalizeReferenceForScope_PathInsideScope(t *testing.T) {
	dir := t.TempDir()
	file := lewpath.New(dir, "doc.go").String()

	ref := ingest.ParseReference(file + "::newDocCmd")
	norm := ingest.NormalizeReferenceForScope(dir, dir, ref)
	require.False(t, norm.Provider != "path" || norm.Path != "./doc.go" || norm.Name != "newDocCmd",
		"unexpected normalized ref: %+v", norm)

}

func TestNormalizeReferenceForScope_PathOutsideScope(t *testing.T) {
	dir := t.TempDir()
	other := lewpath.New(t.TempDir(), "doc.go").String()

	ref := ingest.ParseReference(other + "::newDocCmd")
	norm := ingest.NormalizeReferenceForScope(dir, dir, ref)
	require.Equal(t, ref.Path, norm.Path,
		"expected unchanged path for outside ref, got %q want %q", norm.Path, ref.Path)

}

func TestAbsolutePathReferenceForScope(t *testing.T) {
	tmp := t.TempDir()
	scope := ingest.ReferenceScope{
		Dir:       tmp,
		Reference: ingest.ParseReference("path:./file.go::name"),
	}
	absRef := ingest.AbsolutePathReferenceForScope(scope)

	want := lewpath.New(tmp, "file.go").String()
	require.False(t, absRef.Provider != "path" || absRef.Path != want || absRef.Name != "name",
		"unexpected absolute ref: %+v", absRef)

}
