package ingest_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"
)

func TestPatchFS_WriteOverridesParent(t *testing.T) {
	parent := fstest.MapFS{
		"a.go": {Data: []byte("old")},
		"b.go": {Data: []byte("keep")},
	}
	p := project.NewPatchFS(parent, map[string][]byte{"a.go": []byte("new")})
	got, err := fs.ReadFile(p, "a.go")
	require.NoError(t, err)
	require.Equal(t, "new", string(got),
		"got %q", got)

	keep, err := fs.ReadFile(p, "b.go")
	require.NoError(t, err)
	require.Equal(t, "keep", string(keep),
		"keep %q", keep)

}

func TestPatchFS_FileRename(t *testing.T) {
	parent := fstest.MapFS{"a.go": {Data: []byte("pkg")}}
	p := project.NewPatchFSLayer(parent, nil, map[string]string{"a.go": "b.go"})
	{
		_, err := fs.ReadFile(p, "a.go")
		require.Error(t, err,
			"old name must be gone")
	}

	got, err := fs.ReadFile(p, "b.go")
	require.NoError(t, err)
	require.Equal(t, "pkg", string(got),
		"got %q", got)

}
