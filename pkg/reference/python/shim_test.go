package pythonref

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
)

func TestIsMiseOrUvPythonShim(t *testing.T) {
	dir := t.TempDir()
	shim := lewpath.New(dir, "python3").String()
	body := "#!/usr/bin/env bash\n\nexec mise exec uv -- uv run python3 \"$@\"\n"
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isMiseOrUvPythonShim(shim) {
		t.Fatal("expected mise/uv shim detected")
	}
	realish := lewpath.New(dir, "python-bin").String()
	// pretend binary: large non-shebang
	big := make([]byte, 5000)
	big[0] = 0x7f
	if err := os.WriteFile(realish, big, 0o755); err != nil {
		t.Fatal(err)
	}
	if isMiseOrUvPythonShim(realish) {
		t.Fatal("large binary should not be shim")
	}
}
