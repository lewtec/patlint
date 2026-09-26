package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
)

func TestNodeProvider_ResolveFromNodeModules(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(lewpath.New(root, "node_modules", "react").String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "node_modules", "react", "index.js").String(), []byte("export function createElement() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "node_modules", "react", "package.json").String(), []byte(`{"main":"index.js"}`), 0644); err != nil {
		t.Fatal(err)
	}

	provider, ok := referenceProviderForName("node")
	if !ok {
		t.Fatal("expected node provider to be registered")
	}
	ref, ok := provider.Resolve("react", ImportResolveContext{
		RootDir:      root,
		ImporterPath: "src/main.js",
	})
	if !ok {
		t.Fatal("expected provider to resolve")
	}
	if ref != "path:./node_modules/react/index.js" {
		t.Fatalf("unexpected reference: %q", ref)
	}
}

func TestNodeProvider_ResolveExportsField(t *testing.T) {
	root := t.TempDir()
	pkgDir := lewpath.New(root, "node_modules", "@astrojs", "svelte").String()
	if err := os.MkdirAll(lewpath.New(pkgDir, "dist").String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkgDir, "dist", "index.js").String(), []byte("export default function svelte() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkgDir, "package.json").String(), []byte(`{
		"type": "module",
		"exports": { ".": "./dist/index.js", "./editor": "./dist/editor.cjs" }
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	provider, ok := referenceProviderForName("node")
	if !ok {
		t.Fatal("expected node provider to be registered")
	}
	ref, ok := provider.Resolve("@astrojs/svelte", ImportResolveContext{
		RootDir:      root,
		ImporterPath: "astro.config.mjs",
	})
	if !ok {
		t.Fatal("expected provider to resolve")
	}
	if ref != "path:./node_modules/@astrojs/svelte/dist/index.js" {
		t.Fatalf("unexpected reference: %q", ref)
	}
}

func TestNodeProvider_ResolveExportsSubpath(t *testing.T) {
	root := t.TempDir()
	pkgDir := lewpath.New(root, "node_modules", "astro").String()
	if err := os.MkdirAll(lewpath.New(pkgDir, "dist").String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkgDir, "dist", "config.js").String(), []byte("export function defineConfig() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkgDir, "package.json").String(), []byte(`{
		"type": "module",
		"exports": { ".": "./dist/index.js", "./config": "./dist/config.js" }
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	provider, ok := referenceProviderForName("node")
	if !ok {
		t.Fatal("expected node provider to be registered")
	}
	ref, ok := provider.Resolve("astro/config", ImportResolveContext{
		RootDir:      root,
		ImporterPath: "astro.config.mjs",
	})
	if !ok {
		t.Fatal("expected provider to resolve")
	}
	if ref != "path:./node_modules/astro/dist/config.js" {
		t.Fatalf("unexpected reference: %q", ref)
	}
}

func TestNodeProvider_ResolveExportsConditions(t *testing.T) {
	root := t.TempDir()
	pkgDir := lewpath.New(root, "node_modules", "zod").String()
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkgDir, "index.js").String(), []byte("export const z = {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkgDir, "index.cjs").String(), []byte("module.exports = {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkgDir, "package.json").String(), []byte(`{
		"type": "module",
		"main": "./index.cjs",
		"module": "./index.js",
		"exports": {
			".": { "types": "./index.d.ts", "import": "./index.js", "require": "./index.cjs" }
		}
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	provider, ok := referenceProviderForName("node")
	if !ok {
		t.Fatal("expected node provider to be registered")
	}
	ref, ok := provider.Resolve("zod", ImportResolveContext{
		RootDir:      root,
		ImporterPath: "src/main.js",
	})
	if !ok {
		t.Fatal("expected provider to resolve")
	}
	// Prefer "import" condition over main/require.
	if ref != "path:./node_modules/zod/index.js" {
		t.Fatalf("unexpected reference: %q", ref)
	}
}

func TestNodeProvider_ResolveModuleField(t *testing.T) {
	root := t.TempDir()
	pkgDir := lewpath.New(root, "node_modules", "legacy-esm").String()
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkgDir, "esm.js").String(), []byte("export default 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkgDir, "cjs.js").String(), []byte("module.exports = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// No exports field: module preferred over main.
	if err := os.WriteFile(lewpath.New(pkgDir, "package.json").String(), []byte(`{
		"main": "cjs.js",
		"module": "esm.js"
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	provider, ok := referenceProviderForName("node")
	if !ok {
		t.Fatal("expected node provider to be registered")
	}
	ref, ok := provider.Resolve("legacy-esm", ImportResolveContext{
		RootDir:      root,
		ImporterPath: "src/main.js",
	})
	if !ok {
		t.Fatal("expected provider to resolve")
	}
	if ref != "path:./node_modules/legacy-esm/esm.js" {
		t.Fatalf("unexpected reference: %q", ref)
	}
}

func TestNodeProvider_ResolveExportsPattern(t *testing.T) {
	root := t.TempDir()
	pkgDir := lewpath.New(root, "node_modules", "pkg").String()
	configsDir := lewpath.New(pkgDir, "configs").String()
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(configsDir, "base.json").String(), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(pkgDir, "package.json").String(), []byte(`{
		"exports": { "./tsconfigs/*.json": "./configs/*.json" }
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	provider, ok := referenceProviderForName("node")
	if !ok {
		t.Fatal("expected node provider to be registered")
	}
	ref, ok := provider.Resolve("pkg/tsconfigs/base.json", ImportResolveContext{
		RootDir:      root,
		ImporterPath: "src/main.js",
	})
	if !ok {
		t.Fatal("expected provider to resolve patterned export")
	}
	if ref != "path:./node_modules/pkg/configs/base.json" {
		t.Fatalf("unexpected reference: %q", ref)
	}
}

func TestNodeProvider_ResolveFromNODE_PATH(t *testing.T) {
	root := t.TempDir()
	nodePath := t.TempDir()
	if err := os.MkdirAll(lewpath.New(nodePath, "react").String(), 0755); err != nil {
		t.Fatal(err)
	}
	moduleFile := lewpath.New(nodePath, "react", "index.js").String()
	if err := os.WriteFile(moduleFile, []byte("export function createElement() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(nodePath, "react", "package.json").String(), []byte(`{"main":"index.js"}`), 0644); err != nil {
		t.Fatal(err)
	}

	old := os.Getenv("NODE_PATH")
	t.Cleanup(func() { _ = os.Setenv("NODE_PATH", old) })
	if err := os.Setenv("NODE_PATH", nodePath); err != nil {
		t.Fatal(err)
	}

	provider, ok := referenceProviderForName("node")
	if !ok {
		t.Fatal("expected node provider to be registered")
	}
	ref, ok := provider.Resolve("react", ImportResolveContext{
		RootDir:      root,
		ImporterPath: "src/main.js",
	})
	if !ok {
		t.Fatal("expected provider to resolve")
	}

	expectedSuffix := filepath.ToSlash(moduleFile)
	if !strings.HasPrefix(ref, "path:") || !strings.HasSuffix(ref, expectedSuffix) {
		t.Fatalf("expected absolute path reference ending with %q, got %q", expectedSuffix, ref)
	}
}

func TestNodeProvider_FallbackSymbolic(t *testing.T) {
	root := t.TempDir()
	provider, ok := referenceProviderForName("node")
	if !ok {
		t.Fatal("expected node provider to be registered")
	}
	ref, ok := provider.Resolve("totally-missing-pkg", ImportResolveContext{
		RootDir:      root,
		ImporterPath: "main.js",
	})
	if !ok {
		t.Fatal("expected provider result")
	}
	if ref != "node:totally-missing-pkg" {
		t.Fatalf("unexpected fallback ref: %q", ref)
	}
}

func TestNodeProvider_BuiltinNodeProtocol(t *testing.T) {
	root := t.TempDir()
	provider, ok := referenceProviderForName("node")
	if !ok {
		t.Fatal("expected node provider to be registered")
	}
	// Import specifier is the Node builtin protocol ("node:url"). The provider is
	// also "node", so the path must retain "node:url" → full ref "node:node:url".
	ref, ok := provider.Resolve("node:url", ImportResolveContext{
		RootDir:      root,
		ImporterPath: "astro.config.mjs",
	})
	if !ok {
		t.Fatal("expected provider result")
	}
	if ref != "node:node:url" {
		t.Fatalf("unexpected builtin ref: %q, want node:node:url", ref)
	}

	parsed := ParseReference(ref)
	if parsed.Provider != "node" || parsed.Path != "node:url" {
		t.Fatalf("parsed ref = provider=%q path=%q, want provider=node path=node:url", parsed.Provider, parsed.Path)
	}
}
