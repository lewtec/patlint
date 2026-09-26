package ingest_test

import (
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestWriteBack_LastWriteWins(t *testing.T) {
	dir := t.TempDir()
	{
		err := os.WriteFile(lewpath.New(dir, "a.go").String(), []byte("one"), 0o644)
		require.NoError(t, err)
	}

	parent := fstest.MapFS{"a.go": {Data: []byte("one")}}
	mid := project.NewPatchFS(parent, map[string][]byte{"a.go": []byte("two")})
	top := project.NewPatchFS(mid, map[string][]byte{"a.go": []byte("three")})
	sess := &project.Session{Root: dir, FS: top}
	{
		err := ingest.WriteBack(t.Context(), sess)
		require.NoError(t, err)
	}

	got, err := os.ReadFile(lewpath.New(dir, "a.go").String())
	require.NoError(t, err)
	require.Equal(t, "three", string(got),
		"got %q", got)

}

func TestWriteBack_EmptyNoop(t *testing.T) {
	dir := t.TempDir()
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	err := ingest.WriteBack(t.Context(), sess)
	require.NoError(t, err)

}

func TestWriteBack_FileRename(t *testing.T) {
	dir := t.TempDir()
	{
		err := os.WriteFile(lewpath.New(dir, "a.go").String(), []byte("pkg"), 0o644)
		require.NoError(t, err)
	}

	parent := os.DirFS(dir)
	layer := project.NewPatchFSLayer(parent, nil, map[string]string{"a.go": "b.go"})
	sess := &project.Session{Root: dir, FS: layer}
	{
		err := ingest.WriteBack(t.Context(), sess)
		require.NoError(t, err)
	}
	{

		_, err := os.Stat(lewpath.New(dir, "a.go").String())
		require.True(t, os.IsNotExist(err),
			"old file must be gone")
	}

	got, err := os.ReadFile(lewpath.New(dir, "b.go").String())
	require.NoError(t, err)
	require.Equal(t, "pkg", string(got),
		"got %q", got)

}

func TestOverlayWrites_NonPatchEmpty(t *testing.T) {
	n := len(project.OverlayWrites(fstest.MapFS{"a": {Data: []byte("x")}}))
	require.Equal(t, 0, n,
		"n=%d", n)

	_ = fs.FS(nil)
}
