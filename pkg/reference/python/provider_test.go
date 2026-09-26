package pythonref

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"
)

func TestResolveModuleTarget_FromPYTHONPATH(t *testing.T) {
	tmp := t.TempDir()

	moduleName := "fixturemod"
	moduleFile := lewpath.New(tmp, moduleName+".py").String()
	{
		err := os.WriteFile(moduleFile, []byte("def hello():\n    return 1\n"), 0644)
		require.NoError(t, err)
	}

	packageName := "fixturepkg"
	packageDir := lewpath.New(tmp, packageName).String()
	{
		err := os.MkdirAll(packageDir, 0755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(packageDir, "__init__.py").String(), []byte("def hi():\n    return 2\n"), 0644)
		require.NoError(t, err)
	}

	oldPath := os.Getenv("PYTHONPATH")
	joined := tmp
	if oldPath != "" {
		joined = tmp + string(os.PathListSeparator) + oldPath
	}
	{
		err := os.Setenv("PYTHONPATH", joined)
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		if err := os.Setenv("PYTHONPATH", oldPath); err != nil {
			t.Errorf("restore PYTHONPATH: %v", err)
		}
	})

	modTarget, err := ResolveModuleTarget(t.Context(), moduleName, "")
	if err != nil {
		if errors.Is(err, ErrPythonExecutableNotFound) {
			t.Skip(err.Error())
		}
		require.FailNow(t, fmt.Sprintf("resolve module target failed: %v", err))
	}
	require.Equal(t, filepath.Clean(tmp), filepath.Clean(modTarget.Dir),
		"unexpected module dir: got %q want %q", modTarget.Dir, tmp)
	require.Equal(t, moduleName+".py", modTarget.File,
		"unexpected module file: got %q want %q", modTarget.File, moduleName+".py")

	pkgTarget, err := ResolveModuleTarget(t.Context(), packageName, "")
	require.NoError(t, err,
		"resolve package target failed: %v", err)
	require.Equal(t, filepath.Clean(packageDir), filepath.Clean(pkgTarget.Dir),
		"unexpected package dir: got %q want %q", pkgTarget.Dir, packageDir)
	require.Equal(t, "__init__.py", pkgTarget.File,
		"unexpected package file: got %q want %q", pkgTarget.File, "__init__.py")

}

func TestResolveSymbolTarget(t *testing.T) {
	target, ok, err := ResolveSymbolTarget(t.Context(), "os", "path", "")
	if err != nil {
		if errors.Is(err, ErrPythonExecutableNotFound) {
			t.Skip(err.Error())
		}
		require.FailNow(t, fmt.Sprintf("resolve symbol target failed: %v", err))
	}
	require.True(t, ok,
		"expected symbol target to resolve")
	require.Equal(t, "path", target.Name,
		"unexpected symbol: %q", target.Name)
	require.NotEmpty(t, target.Dir,
		"expected non-empty target dir")

}
