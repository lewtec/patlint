package pythonref

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
)

func TestResolveModuleTarget_FromPYTHONPATH(t *testing.T) {
	tmp := t.TempDir()

	moduleName := "fixturemod"
	moduleFile := lewpath.New(tmp, moduleName+".py").String()
	if err := os.WriteFile(moduleFile, []byte("def hello():\n    return 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	packageName := "fixturepkg"
	packageDir := lewpath.New(tmp, packageName).String()
	if err := os.MkdirAll(packageDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(packageDir, "__init__.py").String(), []byte("def hi():\n    return 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PYTHONPATH")
	joined := tmp
	if oldPath != "" {
		joined = tmp + string(os.PathListSeparator) + oldPath
	}
	if err := os.Setenv("PYTHONPATH", joined); err != nil {
		t.Fatal(err)
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
		t.Fatalf("resolve module target failed: %v", err)
	}
	if filepath.Clean(modTarget.Dir) != filepath.Clean(tmp) {
		t.Fatalf("unexpected module dir: got %q want %q", modTarget.Dir, tmp)
	}
	if modTarget.File != moduleName+".py" {
		t.Fatalf("unexpected module file: got %q want %q", modTarget.File, moduleName+".py")
	}

	pkgTarget, err := ResolveModuleTarget(t.Context(), packageName, "")
	if err != nil {
		t.Fatalf("resolve package target failed: %v", err)
	}
	if filepath.Clean(pkgTarget.Dir) != filepath.Clean(packageDir) {
		t.Fatalf("unexpected package dir: got %q want %q", pkgTarget.Dir, packageDir)
	}
	if pkgTarget.File != "__init__.py" {
		t.Fatalf("unexpected package file: got %q want %q", pkgTarget.File, "__init__.py")
	}
}

func TestResolveSymbolTarget(t *testing.T) {
	target, ok, err := ResolveSymbolTarget(t.Context(), "os", "path", "")
	if err != nil {
		if errors.Is(err, ErrPythonExecutableNotFound) {
			t.Skip(err.Error())
		}
		t.Fatalf("resolve symbol target failed: %v", err)
	}
	if !ok {
		t.Fatal("expected symbol target to resolve")
	}
	if target.Name != "path" {
		t.Fatalf("unexpected symbol: %q", target.Name)
	}
	if target.Dir == "" {
		t.Fatal("expected non-empty target dir")
	}
}
