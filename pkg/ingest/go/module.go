package ingestgo

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
	refpkg "github.com/lewtec/patlint/pkg/reference"
	goref "github.com/lewtec/patlint/pkg/reference/go"
)

// FamilyID is the Go import-resolver id. It matches (as-family "go" …).
const FamilyID = "go"

// Family is the Go family handle (package ≡ directory). Surfaces: go, templ.
var Family = ingest.RegisterFamily(FamilyID, ingest.FamilySpec{
	ResolveImport: ResolveImport,
})

func init() {
	ingest.RegisterReferenceProvider("go", referenceProvider{})
}

// ResolveImport maps an import path/spec to a product reference (go/path or path:./…).
// Shared with templ and other Go-family surfaces.
func ResolveImport(sourcePath string, ctx ingest.ImportResolveContext) string {
	spec := strings.TrimSpace(sourcePath)
	// Relative imports resolve against the importer file directory — not a
	// global knownDirs leaf map (which would map every "./pkg" to top-level pkg).
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return resolveGoRelativeImport(spec, ctx.ImporterPath)
	}
	return goref.ResolveImport(spec, ctx.KnownDirs)
}

// resolveGoRelativeImport joins a ./ or ../ import with the importer's directory
// and returns a path:./… reference (directory package form).
func resolveGoRelativeImport(spec, importerPath string) string {
	imp := strings.TrimPrefix(filepath.ToSlash(importerPath), "./")
	impDir := path.Dir(imp)
	if impDir == "." {
		impDir = ""
	}
	joined := path.Clean(path.Join(impDir, spec))
	if joined == "." || joined == "" {
		return "path:./"
	}
	// Guard path.Clean of ".." escaping above project (rare for normal imports).
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return "go:" + spec
	}
	return "path:./" + joined
}

type referenceProvider struct{}

func (referenceProvider) Name() string { return "go" }

func (referenceProvider) Resolve(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	return goref.ResolveImport(spec, ctx.KnownDirs), true
}

func (referenceProvider) ResolveScopeTarget(ctx context.Context, ref ingest.Reference, rootDir string) (ingest.ProviderScopeTarget, bool, error) {
	if ref.Path == "" {
		return ingest.ProviderScopeTarget{}, false, nil
	}
	dir, err := goref.ResolvePackageDir(ctx, ref.Path, rootDir)
	if err != nil {
		return ingest.ProviderScopeTarget{}, true, err
	}
	return ingest.ProviderScopeTarget{Dir: dir}, true, nil
}

func (referenceProvider) ResolveSymbolTarget(ctx context.Context, ref ingest.Reference, rootDir string) (ingest.ProviderSymbolTarget, bool, error) {
	target, ok, err := goref.ResolveSymbolTarget(ctx, ref.Path, ref.Name, rootDir)
	if !ok || err != nil {
		return ingest.ProviderSymbolTarget{}, ok, err
	}
	return ingest.ProviderSymbolTarget{Dir: target.Dir, Name: target.Name}, true, nil
}

func (referenceProvider) ListScopeChildren(ctx context.Context, ref ingest.Reference, rootDir string, includeHidden bool) ([]refpkg.ScopeChild, bool, error) {
	if ref.Path == "" {
		return nil, false, nil
	}
	dir, err := goref.ResolvePackageDir(ctx, ref.Path, rootDir)
	if err != nil {
		return nil, true, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, true, err
	}

	children := make([]refpkg.ScopeChild, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, true, err
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !includeHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if !dirHasGoSources(lewpath.New(dir, name).String()) {
			continue
		}
		children = append(children, refpkg.ScopeChild{
			Ref:  ingest.Reference{Provider: "go", Path: refpkg.JoinProviderPath(ref.Path, name)},
			Kind: refpkg.ScopeChildDir,
		})
	}
	refpkg.SortScopeChildrenByPath(children)
	return children, true, nil
}

func (referenceProvider) ListIngestRecursive(_ ingest.Reference, opts ingest.ListOptions) bool {
	return opts.Recursive
}

func (referenceProvider) AllowListEntity(ctx context.Context, _ ingest.Reference, _ ingest.Reference, entPath, language string, opts ingest.ListOptions) bool {
	if ctx.Err() != nil {
		return false
	}
	if opts.Policy == nil || !opts.Policy.LanguageInFamily(language, FamilyID) {
		return false
	}
	return !strings.HasSuffix(entPath, "_test.go")
}

func (referenceProvider) ListOutputReference(ref ingest.Reference, entRef ingest.Reference) ingest.Reference {
	return ingest.Reference{Provider: ref.Provider, Path: ref.Path, Name: entRef.Name}
}

func (referenceProvider) DocIngestRecursive(ingest.Reference) bool { return false }

func (referenceProvider) AllowDocEntity(ctx context.Context, _ ingest.Reference, _ ingest.Reference, entPath, language string, claims []project.FamilyClaim) bool {
	if ctx.Err() != nil {
		return false
	}
	if !project.LanguageInFamily(claims, language, FamilyID) {
		return false
	}
	return !strings.HasSuffix(entPath, "_test.go")
}

func dirHasGoSources(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".templ") {
			return true
		}
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			return true
		}
	}
	return false
}
