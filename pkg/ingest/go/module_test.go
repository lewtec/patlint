package ingestgo

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingest"
	refpkg "github.com/lewtec/patlint/pkg/reference"
)

func TestReferenceProvider_ListScopeChildren_FiltersNonPackages(t *testing.T) {
	modCache := t.TempDir()
	libDir := lewpath.New(modCache, "github.com", "example", "lib@v1.2.3").String()
	if err := os.MkdirAll(lewpath.New(libDir, "scan").String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lewpath.New(libDir, "doc").String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(libDir, "print.go").String(), []byte("package lib\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(libDir, "scan", "scan.go").String(), []byte("package scan\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(libDir, "doc", "README.txt").String(), []byte("not go\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldModCache := os.Getenv("GOMODCACHE")
	t.Cleanup(func() { _ = os.Setenv("GOMODCACHE", oldModCache) })
	if err := os.Setenv("GOMODCACHE", modCache); err != nil {
		t.Fatal(err)
	}

	children, ok, err := (referenceProvider{}).ListScopeChildren(t.Context(), ingest.ParseReference("go:github.com/example/lib"), "", false)
	if err != nil {
		t.Fatalf("list scope children failed: %v", err)
	}
	if !ok {
		t.Fatal("expected go provider child listing")
	}

	if !hasScopeChild(children, refpkg.ScopeChild{Ref: ingest.ParseReference("go:github.com/example/lib/scan"), Kind: refpkg.ScopeChildDir}) {
		t.Fatalf("expected scan child package, got %+v", children)
	}
	if hasScopeChild(children, refpkg.ScopeChild{Ref: ingest.ParseReference("go:github.com/example/lib/doc"), Kind: refpkg.ScopeChildDir}) {
		t.Fatalf("did not expect non-go directory child, got %+v", children)
	}
}

func hasScopeChild(children []refpkg.ScopeChild, want refpkg.ScopeChild) bool {
	for _, child := range children {
		if child.Kind == want.Kind && child.Ref == want.Ref {
			return true
		}
	}
	return false
}

func TestResolveGoRelativeImport(t *testing.T) {
	cases := []struct {
		spec, importer, want string
	}{
		{"./pkg", "testdata/fixture/input/other.go", "path:./testdata/fixture/input/pkg"},
		{"./pkg", "main.go", "path:./pkg"},
		{"../pkg", "testdata/fixture/input/sub/other.go", "path:./testdata/fixture/input/pkg"},
		{"./pkg/sub", "cmd/tool/main.go", "path:./cmd/tool/pkg/sub"},
	}
	for _, tc := range cases {
		got := resolveGoRelativeImport(tc.spec, tc.importer)
		if got != tc.want {
			t.Errorf("resolveGoRelativeImport(%q, %q)=%q want %q", tc.spec, tc.importer, got, tc.want)
		}
	}
}

func TestResolveImport_RelativeUsesImporter(t *testing.T) {
	ctx := ingest.ImportResolveContext{
		ImporterPath: "testdata/fixture/input/other.go",
		KnownDirs:    map[string]bool{"pkg": true}, // top-level pkg must not win
	}
	got := ResolveImport("./pkg", ctx)
	want := "path:./testdata/fixture/input/pkg"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
