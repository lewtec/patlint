package pythonref

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"
)

func TestIsMiseOrUvPythonShim(t *testing.T) {
	dir := t.TempDir()
	shim := lewpath.New(dir, "python3").String()
	body := "#!/usr/bin/env bash\n\nexec mise exec uv -- uv run python3 \"$@\"\n"
	{
		err := os.WriteFile(shim, []byte(body), 0o755)
		require.NoError(t, err)
	}
	require.True(t, isMiseOrUvPythonShim(shim),
		"expected mise/uv shim detected")

	realish := lewpath.New(dir, "python-bin").String()
	// pretend binary: large non-shebang
	big := make([]byte, 5000)
	big[0] = 0x7f
	err := os.WriteFile(realish, big, 0o755)
	require.NoError(t, err)
	require.False(t, isMiseOrUvPythonShim(realish),
		"large binary should not be shim")

}
