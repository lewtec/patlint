package ingest

import (
	"errors"
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	pythonref "github.com/lewtec/patlint/pkg/reference/python"
)

func TestPythonProvider_ResolveAbsoluteModule_LocalFile(t *testing.T) {
	provider, ok := referenceProviderForName("python")
	if !ok {
		t.Fatal("expected python provider to be registered")
	}

	ref, ok := provider.Resolve("pkg.sub", ImportResolveContext{
		KnownFiles: map[string]bool{
			"pkg/sub.py": true,
		},
	})
	if !ok {
		t.Fatal("expected provider to resolve")
	}
	if ref != "path:./pkg/sub.py" {
		t.Fatalf("unexpected reference: %q", ref)
	}
}

func TestPythonProvider_ResolveAbsoluteModule_FallbackProvider(t *testing.T) {
	provider, ok := referenceProviderForName("python")
	if !ok {
		t.Fatal("expected python provider to be registered")
	}

	ref, ok := provider.Resolve("os.path", ImportResolveContext{
		KnownFiles: map[string]bool{},
	})
	if !ok {
		t.Fatal("expected provider result")
	}
	if ref != "python:os.path" {
		t.Fatalf("unexpected fallback reference: %q", ref)
	}
}

func TestPythonProvider_ResolveRelativeImport_CurrentPackage(t *testing.T) {
	provider, ok := referenceProviderForName("python")
	if !ok {
		t.Fatal("expected python provider to be registered")
	}

	ref, ok := provider.Resolve(".localmod", ImportResolveContext{
		ImporterPath: "pkg/app.py",
		KnownFiles: map[string]bool{
			"pkg/localmod.py": true,
		},
	})
	if !ok {
		t.Fatal("expected provider to resolve")
	}
	if ref != "path:./pkg/localmod.py" {
		t.Fatalf("unexpected relative import reference: %q", ref)
	}
}

func TestPythonProvider_ResolveRelativeImport_ParentPackage(t *testing.T) {
	provider, ok := referenceProviderForName("python")
	if !ok {
		t.Fatal("expected python provider to be registered")
	}

	ref, ok := provider.Resolve("..parentpkg", ImportResolveContext{
		ImporterPath: "pkg/sub/app.py",
		KnownFiles: map[string]bool{
			"pkg/parentpkg.py": true,
		},
	})
	if !ok {
		t.Fatal("expected provider to resolve")
	}
	if ref != "path:./pkg/parentpkg.py" {
		t.Fatalf("unexpected parent relative reference: %q", ref)
	}
}

func TestPythonProvider_ResolveScopeTarget_CanDescendByModuleKind(t *testing.T) {
	if _, ok := referenceProviderForName("python"); !ok {
		t.Fatal("expected python provider to be registered")
	}

	tmp := t.TempDir()
	moduleName := "fixturemodscope"
	packageName := "fixturepkgscope"

	if err := os.WriteFile(lewpath.New(tmp, moduleName+".py").String(), []byte("def a():\n    return 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lewpath.New(tmp, packageName).String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(tmp, packageName, "__init__.py").String(), []byte("def b():\n    return 2\n"), 0644); err != nil {
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
		_ = os.Setenv("PYTHONPATH", oldPath)
	})

	r := NewResolver("")
	moduleTarget, ok, err := r.ResolveScopeTarget(t.Context(), ParseReference("python:"+moduleName))
	if err != nil {
		if errors.Is(err, pythonref.ErrPythonExecutableNotFound) {
			t.Skip(err.Error())
		}
		t.Fatalf("resolve module scope failed: %v", err)
	}
	if !ok {
		t.Fatal("expected module scope target")
	}
	if moduleTarget.CanDescend == nil || *moduleTarget.CanDescend {
		t.Fatalf("expected module scope to disable child navigation, got %+v", moduleTarget)
	}

	packageTarget, ok, err := r.ResolveScopeTarget(t.Context(), ParseReference("python:"+packageName))
	if err != nil {
		t.Fatalf("resolve package scope failed: %v", err)
	}
	if !ok {
		t.Fatal("expected package scope target")
	}
	if packageTarget.CanDescend == nil || !*packageTarget.CanDescend {
		t.Fatalf("expected package scope to enable child navigation, got %+v", packageTarget)
	}
}
