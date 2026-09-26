package js

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingest/ecma"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

// ErrUnknownGrammar is tabled for empty or unknown grammar ids.
var ErrUnknownGrammar = errors.New("unknown grammar")

// ECMA family surfaces under pkg/ingest/ecma:
//   - javascript, typescript, tsx — host ids from prelude path globs
//   - svelte, vue, astro — separate surface ids
// Extract/move/resolve are shared leftovers; language id comes from packs.

func init() {
	ingest.RegisterFamily(ecma.FamilyID, ingest.FamilySpec{
		Lattice:       ecma.Family.Lattice(),
		ResolveImport: ResolveECMAImport,
	})
	ingest.RegisterReferenceProvider("node", referenceProvider{})
	ingest.RegisterDirectoryManifestResolve(resolvePackageJSONDir)
}

func resolvePackageJSONDir(absDir string) (string, bool) {
	if resolved, ok := resolvePackageEntrypoint(absDir, "", ingest.ImportResolveContext{}); ok {
		st, err := os.Stat(absDir)
		if err != nil || !st.IsDir() {
			return "", false
		}
		rel, err := filepath.Rel(absDir, resolved)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return "", false
		}
		return filepath.ToSlash(rel), true
	}
	return "", false
}

type referenceProvider struct{}

func (referenceProvider) Name() string { return "node" }

func (referenceProvider) HighlightLanguage() string {
	return "javascript"
}

func (referenceProvider) Resolve(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	return resolveNodeImport(spec, ctx)
}

func resolvePathImport(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	if !(strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") || strings.HasPrefix(spec, "/")) {
		return "", false
	}

	rel := ingest.RelativeImportPath(ctx.ImporterPath, spec)
	if rel == "" {
		return "", false
	}

	if ref, ok := resolveKnownJSPath(rel, ctx); ok {
		return ref, true
	}

	rootAbs, err := filepath.Abs(ctx.RootDir)
	if err != nil {
		return ingest.FileRef("./" + rel), true
	}
	candidate := lewpath.New(rootAbs, filepath.FromSlash(rel)).String()
	resolved, ok := resolveJSFileOnDisk(candidate, false, rel, ctx)
	if ok {
		return ingest.PathReferenceForAbsolute(rootAbs, resolved), true
	}

	return ingest.FileRef("./" + rel), true
}

// nodeSymbolicRef builds a node-provider reference for an unresolved import.
// Always prefixes with the provider ("node:"). Specifiers that already use the
// Node builtin protocol (e.g. "node:url") therefore become "node:node:url"
// (provider=node, path=node:url), not "node:url" (provider=node, path=url).
func nodeSymbolicRef(spec string) string {
	return "node:" + spec
}

func resolveNodeImport(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	if strings.HasPrefix(spec, "node:") {
		// Node builtin / explicit protocol — keep full "node:..." as the path.
		return nodeSymbolicRef(spec), true
	}
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") || strings.HasPrefix(spec, "/") {
		return "", false
	}

	pkgName, subpath := splitNodePackageSpecifier(spec)
	if pkgName == "" {
		return nodeSymbolicRef(spec), true
	}

	rootAbs, err := filepath.Abs(ctx.RootDir)
	if err != nil {
		rootAbs = ctx.RootDir
	}
	importerAbs := lewpath.New(rootAbs, filepath.FromSlash(filepath.Dir(ctx.ImporterPath))).String()

	for _, pkgRoot := range nodeModuleCandidates(importerAbs, pkgName) {
		if st, err := os.Stat(pkgRoot); err != nil || !st.IsDir() {
			continue
		}

		// Prefer package.json entrypoints (exports / module / main) — same order Node uses.
		if resolved, ok := resolvePackageEntrypoint(pkgRoot, subpath, ctx); ok {
			return ingest.PathReferenceForAbsolute(rootAbs, resolved), true
		}

		// Fallback when there is no package.json (or no usable entry): treat as a plain path.
		targetBase := pkgRoot
		if subpath != "" {
			targetBase = lewpath.New(pkgRoot, filepath.FromSlash(subpath)).String()
		}
		if resolved, ok := resolveJSFileOnDisk(targetBase, false, subpath, ctx); ok {
			return ingest.PathReferenceForAbsolute(rootAbs, resolved), true
		}
		if st, err := os.Stat(targetBase); err == nil && st.IsDir() {
			return ingest.PathReferenceForAbsolute(rootAbs, targetBase), true
		}
	}

	return nodeSymbolicRef(spec), true
}

func resolveKnownJSPath(rel string, ctx ingest.ImportResolveContext) (string, bool) {
	if ctx.KnownFiles[rel] {
		return ingest.FileRef("./" + rel), true
	}
	for _, ext := range ecmaResolveExtensions {
		if ctx.KnownFiles[rel+ext] {
			return ingest.FileRef("./" + rel + ext), true
		}
	}
	if child, ok := ingest.KnownDirRepresentant(ctx.Representant, rel, ctx.KnownFiles); ok {
		return ingest.FileRef("./" + child), true
	}
	return "", false
}

// ecmaResolveExtensions is the order ECMA/TS-style resolution tries for bare paths.
var ecmaResolveExtensions = []string{".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx"}

func resolveJSFileOnDisk(baseAbs string, preferPackageMain bool, dirRel string, ctx ingest.ImportResolveContext) (string, bool) {
	if st, err := os.Stat(baseAbs); err == nil {
		if !st.IsDir() {
			return baseAbs, true
		}
		if preferPackageMain {
			if resolved, ok := resolvePackageEntrypoint(baseAbs, "", ctx); ok {
				return resolved, true
			}
		}
		if base, ok := ingest.DiskDirRepresentant(ctx.Representant, baseAbs, dirRel); ok {
			return lewpath.New(baseAbs, filepath.FromSlash(base)).String(), true
		}
	}
	for _, ext := range ecmaResolveExtensions {
		candidate := baseAbs + ext
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// packageJSON mirrors the fields Node consults when resolving a package entrypoint.
type packageJSON struct {
	Main    string          `json:"main"`
	Module  string          `json:"module"`
	Browser json.RawMessage `json:"browser"`
	Exports json.RawMessage `json:"exports"`
}

// resolvePackageEntrypoint resolves a Node package root (or subpath within it) using
// package.json conventions: exports (preferred), then module, then main, then index.*.
// subpath is without a leading "./" (e.g. "" for the package root, "config" for astro/config).
func resolvePackageEntrypoint(pkgRoot, subpath string, ctx ingest.ImportResolveContext) (string, bool) {
	pkgPath := lewpath.New(pkgRoot, "package.json").String()
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return "", false
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", false
	}

	// 1. exports field — modern Node resolution (takes precedence when present; main/module ignored).
	if len(pkg.Exports) > 0 {
		if entry, ok := resolveExportsEntry(pkg.Exports, subpath); ok {
			return resolvePackageRelative(pkgRoot, entry, ctx)
		}
		// exports is authoritative: unexported subpaths (and missing ".") fail, no main/module fallback.
		return "", false
	}

	// Subpaths without exports: fall back to filesystem (legacy packages).
	if subpath != "" {
		target := lewpath.New(pkgRoot, filepath.FromSlash(subpath)).String()
		return resolveJSFileOnDisk(target, false, subpath, ctx)
	}

	// 2. module (ESM entry; common in browser/bundler packages).
	if pkg.Module != "" {
		if resolved, ok := resolvePackageRelative(pkgRoot, pkg.Module, ctx); ok {
			return resolved, true
		}
	}

	// 3. main (CommonJS / Node classic entry).
	if pkg.Main != "" {
		if resolved, ok := resolvePackageRelative(pkgRoot, pkg.Main, ctx); ok {
			return resolved, true
		}
	}

	// 4. browser field as a string (simple redirect; object form is ignored).
	if len(pkg.Browser) > 0 {
		var browserStr string
		if err := json.Unmarshal(pkg.Browser, &browserStr); err == nil && browserStr != "" {
			if resolved, ok := resolvePackageRelative(pkgRoot, browserStr, ctx); ok {
				return resolved, true
			}
		}
	}

	if base, ok := ingest.DiskDirRepresentant(ctx.Representant, pkgRoot, subpath); ok {
		return lewpath.New(pkgRoot, filepath.FromSlash(base)).String(), true
	}
	return "", false
}

func resolvePackageRelative(pkgRoot, entry string, ctx ingest.ImportResolveContext) (string, bool) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return "", false
	}
	// package.json entries are package-relative; strip optional "./".
	entry = strings.TrimPrefix(entry, "./")
	abs := lewpath.New(pkgRoot, filepath.FromSlash(entry)).String()
	return resolveJSFileOnDisk(abs, false, entry, ctx)
}

// resolveExportsEntry picks a target string from package.json "exports" for the given
// import subpath ("" means package root / ".").
func resolveExportsEntry(exportsRaw json.RawMessage, subpath string) (string, bool) {
	// exports can be a string: "." only.
	var asString string
	if err := json.Unmarshal(exportsRaw, &asString); err == nil {
		if subpath == "" && asString != "" {
			return asString, true
		}
		return "", false
	}

	// exports as a map keyed by subpath (".", "./config", ...) or condition object for root.
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(exportsRaw, &asMap); err != nil {
		return "", false
	}

	key := "."
	if subpath != "" {
		key = "./" + subpath
	}

	// Direct key match.
	if raw, ok := asMap[key]; ok {
		return pickExportTarget(raw)
	}

	// Root request with a condition-only exports object (no "." key, keys are conditions).
	if subpath == "" {
		if target, ok := pickExportTarget(exportsRaw); ok {
			return target, true
		}
	}

	// Pattern keys like "./tsconfigs/*.json" — support a single "*" segment.
	for pattern, raw := range asMap {
		if !strings.Contains(pattern, "*") {
			continue
		}
		matched, ok := matchExportPattern(pattern, key)
		if !ok {
			continue
		}
		target, ok := pickExportTarget(raw)
		if !ok {
			continue
		}
		target = strings.Replace(target, "*", matched, 1)
		return target, true
	}

	return "", false
}

// pickExportTarget resolves an exports value which may be a string or a conditions object.
// Prefers ESM-oriented conditions for static import analysis.
func pickExportTarget(raw json.RawMessage) (string, bool) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil && asString != "" {
		return asString, true
	}

	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return "", false
	}

	// Condition priority for import/static analysis (skip types — we want runtime JS).
	for _, cond := range []string{"import", "default", "node", "require", "module", "browser", "worker"} {
		if nested, ok := asMap[cond]; ok {
			if target, ok := pickExportTarget(nested); ok {
				return target, true
			}
		}
	}

	// Last resort: any non-types string value in the map (depth-1).
	for k, nested := range asMap {
		if k == "types" {
			continue
		}
		var s string
		if err := json.Unmarshal(nested, &s); err == nil && s != "" {
			return s, true
		}
	}

	return "", false
}

// matchExportPattern matches an exports key pattern (single "*") against a request key.
// Returns the substring captured by "*" when it matches.
func matchExportPattern(pattern, request string) (string, bool) {
	star := strings.Index(pattern, "*")
	if star < 0 {
		return "", false
	}
	prefix := pattern[:star]
	suffix := pattern[star+1:]
	if !strings.HasPrefix(request, prefix) || !strings.HasSuffix(request, suffix) {
		return "", false
	}
	mid := request[len(prefix) : len(request)-len(suffix)]
	// Empty capture is only valid when the pattern explicitly allows it (prefix ends with "/").
	if mid == "" && !strings.HasSuffix(prefix, "/") {
		return "", false
	}
	return mid, true
}

func splitNodePackageSpecifier(spec string) (pkgName, subpath string) {
	parts := strings.Split(spec, "/")
	if spec == "" {
		return "", ""
	}
	if strings.HasPrefix(spec, "@") {
		if len(parts) < 2 {
			return spec, ""
		}
		pkgName = parts[0] + "/" + parts[1]
		if len(parts) > 2 {
			subpath = strings.Join(parts[2:], "/")
		}
		return pkgName, subpath
	}
	pkgName = parts[0]
	if len(parts) > 1 {
		subpath = strings.Join(parts[1:], "/")
	}
	return pkgName, subpath
}

func nodeModuleCandidates(importerAbsDir, pkgName string) []string {
	seen := map[string]bool{}
	out := []string{}

	for dir := importerAbsDir; ; {
		candidate := lewpath.New(dir, "node_modules", filepath.FromSlash(pkgName)).String()
		if !seen[candidate] {
			seen[candidate] = true
			out = append(out, candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if nodePath := os.Getenv("NODE_PATH"); nodePath != "" {
		for _, entry := range filepath.SplitList(nodePath) {
			if entry == "" {
				continue
			}
			candidate := lewpath.New(entry, filepath.FromSlash(pkgName)).String()
			if !seen[candidate] {
				seen[candidate] = true
				out = append(out, candidate)
			}
		}
	}

	return out
}

// ExtractECMAScript parses script with grammarName and runs the language extract pack.
// Offsets are relative to script (SFC merge shifts them onto the host file).
func ExtractECMAScript(ctx context.Context, policy ingest.PackQueries, script []byte, grammarName, relPath string) (*project.FileExtract, error) {
	if grammarName == "" {
		return nil, fmt.Errorf("%w %q", ErrUnknownGrammar, grammarName)
	}
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, script, grammarName, grammarName)
	if err != nil {
		return nil, err
	}
	defer pf.Close()
	fe := &project.FileExtract{Language: grammarName, Path: relPath}
	if x, err := ingest.ExtractFile(ctx, policy, nil, grammarName, pf.Root, script, relPath); err == nil && x != nil {
		fe = x
		fe.Language = grammarName
		fe.Path = relPath
	}
	return fe, nil
}

// ResolveECMAImport resolves a path or node module specifier using ECMA rules.
func ResolveECMAImport(sourcePath string, ctx ingest.ImportResolveContext) string {
	if ref, ok := resolvePathImport(sourcePath, ctx); ok {
		return ref
	}
	if ref, ok := resolveNodeImport(sourcePath, ctx); ok {
		return ref
	}
	return nodeSymbolicRef(sourcePath)
}

// ExtractECMAExpressionUsages runs the JS/TS extract pack on a short expression.
func ExtractECMAExpressionUsages(ctx context.Context, policy ingest.PackQueries, expr []byte, grammarName string) ([]project.UsageDef, error) {
	if len(bytes.TrimSpace(expr)) == 0 {
		return nil, nil
	}
	if grammarName == "" {
		return nil, fmt.Errorf("%w %q", ErrUnknownGrammar, grammarName)
	}
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, expr, grammarName, grammarName)
	if err != nil {
		return nil, err
	}
	defer pf.Close()
	fe, err := ingest.ExtractFile(ctx, policy, nil, grammarName, pf.Root, expr, "expr.js")
	if err != nil || fe == nil {
		return nil, err
	}
	return fe.Usages, nil
}
