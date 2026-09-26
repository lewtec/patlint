package reference

import (
	"context"
	"path/filepath"

	pythonref "github.com/lewtec/patlint/pkg/reference/python"
)

// Resolver performs provider-backed scope/symbol resolution in the context of a
// project root (e.g. serve --dir, browse root). Language/provider specifics are
// dispatched through registered providers; only this type carries RootDir.
type Resolver struct {
	// RootDir is the project/module working directory used by providers that
	// run tools like `go list` / project .venv python (empty = no project context).
	RootDir string
}

// NewResolver builds a resolver for the given project root. Non-absolute paths
// are cleaned; errors are not returned here so callers can always construct one.
// Non-empty root is also registered as the default python project root so
// resolution prefers that tree's .venv even when a call site passes workDir "".
func NewResolver(rootDir string) *Resolver {
	if rootDir != "" {
		if abs, err := filepath.Abs(rootDir); err == nil {
			rootDir = abs
		}
		pythonref.SetDefaultProjectRoot(rootDir)
	}
	return &Resolver{RootDir: rootDir}
}

// ResolveSymbolTarget dispatches to the provider named by ref.Provider.
func (r *Resolver) ResolveSymbolTarget(ctx context.Context, ref Reference) (SymbolTarget, bool, error) {
	if r == nil {
		r = &Resolver{}
	}
	provider, ok := ProviderForName(ref.Provider)
	if !ok {
		return SymbolTarget{}, false, nil
	}
	symbolProvider, ok := provider.(SymbolTargetProvider)
	if !ok {
		return SymbolTarget{}, false, nil
	}
	return symbolProvider.ResolveSymbolTarget(ctx, ref, r.RootDir)
}

// ResolveScopeTarget dispatches to the provider named by ref.Provider.
func (r *Resolver) ResolveScopeTarget(ctx context.Context, ref Reference) (ScopeTarget, bool, error) {
	if r == nil {
		r = &Resolver{}
	}
	provider, ok := ProviderForName(ref.Provider)
	if !ok {
		return ScopeTarget{}, false, nil
	}
	scopeProvider, ok := provider.(ScopeTargetProvider)
	if !ok {
		return ScopeTarget{}, false, nil
	}
	return scopeProvider.ResolveScopeTarget(ctx, ref, r.RootDir)
}

// ResolveScopeChildren dispatches to the provider named by ref.Provider.
func (r *Resolver) ResolveScopeChildren(ctx context.Context, ref Reference, includeHidden bool) ([]ScopeChild, bool, error) {
	if r == nil {
		r = &Resolver{}
	}
	provider, ok := ProviderForName(ref.Provider)
	if !ok {
		return nil, false, nil
	}
	childrenProvider, ok := provider.(ScopeChildrenProvider)
	if !ok {
		return nil, false, nil
	}
	return childrenProvider.ListScopeChildren(ctx, ref, r.RootDir, includeHidden)
}
