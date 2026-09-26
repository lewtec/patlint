package projectfs

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"
)

func TestOverlayReadFile(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "a.go").String()
	require.NoError(t, os.WriteFile(path, []byte("disk"), 0o644))

	o := NewOverlay(nil)
	b, err := o.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "disk", string(b))

	o.SetString(path, "overlay")
	b, err = o.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "overlay", string(b))

	o.Delete(path)
	b, err = o.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "disk", string(b))
}
