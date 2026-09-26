package rust

import (
	"context"
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
	rustref "github.com/lewtec/patlint/pkg/reference/rust"
)

func init() {
	ingest.RegisterReferenceProvider("rust", referenceProvider{})
}

// ResolveImport maps a rust import spec to a product ref.
func ResolveImport(sourcePath string, ctx ingest.ImportResolveContext) string {
	if ref, ok := resolveRustImportSpec(sourcePath, ctx); ok {
		return ref
	}
	return "rust:" + sourcePath
}

// --- reference provider ---

type referenceProvider struct{}

func (referenceProvider) Name() string { return "rust" }

func (referenceProvider) Resolve(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	if ref, ok := resolveRustImportSpec(spec, ctx); ok {
		return ref, true
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}
	return "rust:" + spec, true
}

func (referenceProvider) ResolveScopeTarget(ctx context.Context, ref ingest.Reference, rootDir string) (ingest.ProviderScopeTarget, bool, error) {
	if ref.Path == "" {
		return ingest.ProviderScopeTarget{}, false, nil
	}
	target, err := rustref.ResolveCrateTarget(ref.Path, rootDir)
	if err != nil {
		return ingest.ProviderScopeTarget{}, true, err
	}
	canDescend := target.IsDir
	return ingest.ProviderScopeTarget{Dir: target.Dir, File: target.File, CanDescend: &canDescend}, true, nil
}

func (referenceProvider) ResolveSymbolTarget(ctx context.Context, ref ingest.Reference, rootDir string) (ingest.ProviderSymbolTarget, bool, error) {
	target, ok, err := rustref.ResolveSymbolTarget(ctx, ref.Path, ref.Name, rootDir)
	if !ok || err != nil {
		return ingest.ProviderSymbolTarget{}, ok, err
	}
	return ingest.ProviderSymbolTarget{Dir: target.Dir, Name: target.Name}, true, nil
}

func (referenceProvider) ListIngestRecursive(_ ingest.Reference, opts ingest.ListOptions) bool {
	return opts.Recursive
}

func (referenceProvider) AllowListEntity(_ context.Context, ref ingest.Reference, _ ingest.Reference, entPath, language string, _ ingest.ListOptions) bool {
	return rustAllowEntity(ref, entPath, language)
}

func (referenceProvider) ListOutputReference(ref ingest.Reference, entRef ingest.Reference) ingest.Reference {
	return ingest.Reference{Provider: ref.Provider, Path: ref.Path, Name: entRef.Name}
}

func (referenceProvider) DocIngestRecursive(ingest.Reference) bool { return false }

func (referenceProvider) AllowDocEntity(_ context.Context, ref ingest.Reference, _ ingest.Reference, entPath, language string, _ []project.FamilyClaim) bool {
	return rustAllowEntity(ref, entPath, language)
}

func rustAllowEntity(ref ingest.Reference, entPath, language string) bool {
	if language != "rust" {
		return false
	}
	target, err := rustref.ResolveCrateTarget(ref.Path, "")
	if err != nil {
		return false
	}
	return rustref.MatchesEntityPath(target, entPath)
}
