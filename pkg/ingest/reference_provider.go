package ingest

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/project"
	refpkg "github.com/lewtec/patlint/pkg/reference"
)

type ImportResolveContext = refpkg.ImportResolveContext
type ProviderSymbolTarget = refpkg.SymbolTarget
type ProviderScopeTarget = refpkg.ScopeTarget

type providerListPolicy interface {
	ListIngestRecursive(ref Reference, opts ListOptions) bool
	AllowListEntity(ctx context.Context, ref Reference, entRef Reference, entPath, language string, opts ListOptions) bool
	ListOutputReference(ref Reference, entRef Reference) Reference
}

type providerDocPolicy interface {
	DocIngestRecursive(ref Reference) bool
	AllowDocEntity(ctx context.Context, ref Reference, entRef Reference, entPath, language string, claims []project.FamilyClaim) bool
}

// RegisterReferenceProvider registers a reference provider by name.
// It panics on empty names, nil providers, or duplicate names.
func RegisterReferenceProvider(name string, provider refpkg.Provider) {
	name = strings.ToLower(name)
	if name == "" {
		panic("ingest: RegisterReferenceProvider with empty name")
	}
	if provider == nil {
		panic("ingest: RegisterReferenceProvider with nil provider")
	}
	if provider.Name() != "" && !strings.EqualFold(provider.Name(), name) {
		panic(fmt.Sprintf("ingest: RegisterReferenceProvider name mismatch: key=%q provider=%q", name, provider.Name()))
	}
	if _, exists := refpkg.ProviderForName(name); exists {
		panic(fmt.Sprintf("ingest: reference provider %q already registered", name))
	}
	refpkg.RegisterProvider(name, provider)
}

func referenceProviderForName(name string) (refpkg.Provider, bool) {
	return refpkg.ProviderForName(name)
}

// Resolver is a project-scoped reference resolver (see reference.Resolver).
type Resolver = refpkg.Resolver

// NewResolver constructs a resolver rooted at the project directory.
func NewResolver(rootDir string) *Resolver {
	return refpkg.NewResolver(rootDir)
}

func providerListIngestRecursive(ref Reference, opts ListOptions) bool {
	if ref.Provider == "" || ref.Provider == "path" {
		return opts.Recursive
	}
	provider, ok := referenceProviderForName(ref.Provider)
	if !ok {
		return opts.Recursive
	}
	policy, ok := provider.(providerListPolicy)
	if !ok {
		return opts.Recursive
	}
	return policy.ListIngestRecursive(ref, opts)
}

func providerAllowListAtom(ctx context.Context, ref Reference, entRef Reference, entPath, language string, opts ListOptions) bool {
	if ref.Provider == "" || ref.Provider == "path" {
		return true
	}
	provider, ok := referenceProviderForName(ref.Provider)
	if !ok {
		return true
	}
	policy, ok := provider.(providerListPolicy)
	if !ok {
		return true
	}
	return policy.AllowListEntity(ctx, ref, entRef, entPath, language, opts)
}

func providerListOutputReference(ref Reference, entRef Reference) Reference {
	if ref.Provider == "" || ref.Provider == "path" {
		return entRef
	}
	provider, ok := referenceProviderForName(ref.Provider)
	if ok {
		if policy, ok := provider.(providerListPolicy); ok {
			return policy.ListOutputReference(ref, entRef)
		}
	}
	return Reference{
		Provider: ref.Provider,
		Path:     ref.Path,
		Name:     entRef.Name,
	}
}

func ProviderDocIngestRecursive(ref Reference) bool {
	if ref.Provider == "" || ref.Provider == "path" {
		return true
	}
	provider, ok := referenceProviderForName(ref.Provider)
	if !ok {
		return true
	}
	policy, ok := provider.(providerDocPolicy)
	if !ok {
		return true
	}
	return policy.DocIngestRecursive(ref)
}

func providerAllowDocEntity(ctx context.Context, ref Reference, entRef Reference, entPath, language string, claims []project.FamilyClaim) bool {
	if ref.Provider == "" || ref.Provider == "path" {
		return true
	}
	provider, ok := referenceProviderForName(ref.Provider)
	if !ok {
		return true
	}
	policy, ok := provider.(providerDocPolicy)
	if !ok {
		return true
	}
	return policy.AllowDocEntity(ctx, ref, entRef, entPath, language, claims)
}

// PathReferenceForAbsolute returns a path: reference for absolutePath relative to rootDir when
// possible, otherwise an absolute path: reference.
func PathReferenceForAbsolute(rootDir, absolutePath string) string {
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return FileRef(filepath.ToSlash(absolutePath))
	}
	abs, err := filepath.Abs(absolutePath)
	if err != nil {
		return FileRef(filepath.ToSlash(absolutePath))
	}

	rel, err := filepath.Rel(rootAbs, abs)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return FileRef("./" + filepath.ToSlash(rel))
	}
	return FileRef(filepath.ToSlash(abs))
}

// RelativeImportPath joins a relative import spec against the importer file path.
func RelativeImportPath(importerPath, specifier string) string {
	importerDir := filepath.ToSlash(filepath.Dir(importerPath))
	if importerDir == "." {
		importerDir = ""
	}
	if strings.HasPrefix(specifier, "/") {
		return strings.TrimPrefix(filepath.ToSlash(specifier), "/")
	}
	joined := filepath.ToSlash(filepath.Clean(lewpath.New(importerDir, specifier).String()))
	joined = strings.TrimPrefix(joined, "./")
	return joined
}
