package reference

import (
	"context"
	"strings"
	"sync"
)

// RepresentantMatcher is pack as-directory-representant (caller VM).
type RepresentantMatcher interface {
	DirectoryRepresentants() []string
	MatchDirectoryRepresentant(rel string) bool
}

// ImportResolveContext carries filesystem and index metadata needed by
// provider-backed import resolution.
type ImportResolveContext struct {
	RootDir      string
	ImporterPath string
	KnownFiles   map[string]bool
	KnownDirs    map[string]bool
	Representant RepresentantMatcher
}

// Provider resolves import specs into canonical references.
type Provider interface {
	Name() string
	Resolve(spec string, ctx ImportResolveContext) (string, bool)
}

// SymbolTarget points to a directory that can be ingested for provider-backed
// symbol lookup.
type SymbolTarget struct {
	Dir  string
	Name string
}

// ScopeTarget points to a directory that can be ingested for provider-backed
// scope operations such as listing.
type ScopeTarget struct {
	Dir string
	// File, when non-empty, is a basename under Dir for a single-file module
	// (e.g. os.py under lib/python3.x). Callers must not Dir-walk Dir in that
	// case — that would ingest an entire stdlib / site-packages tree.
	File string
	// CanDescend controls whether provider browse UIs should offer child scope
	// navigation from this target. Nil means unknown/default behavior.
	CanDescend *bool
}

type ScopeChildKind int

const (
	ScopeChildDir ScopeChildKind = iota
	ScopeChildFile
)

// ScopeChild is a provider-backed child scope exposed to browse UIs.
type ScopeChild struct {
	Ref  Reference
	Kind ScopeChildKind
}

// SymbolTargetProvider is an optional provider capability used by doc lookup.
// rootDir is injected by Resolver (project/module context for go list, etc.).
type SymbolTargetProvider interface {
	ResolveSymbolTarget(ctx context.Context, ref Reference, rootDir string) (SymbolTarget, bool, error)
}

// ScopeTargetProvider is an optional provider capability used by listing.
type ScopeTargetProvider interface {
	ResolveScopeTarget(ctx context.Context, ref Reference, rootDir string) (ScopeTarget, bool, error)
}

// ScopeChildrenProvider is an optional provider capability used by browse UIs.
type ScopeChildrenProvider interface {
	ListScopeChildren(ctx context.Context, ref Reference, rootDir string, includeHidden bool) ([]ScopeChild, bool, error)
}

var (
	providersMu sync.RWMutex
	providers   = map[string]Provider{}
)

// RegisterProvider registers or overrides a provider name.
func RegisterProvider(name string, provider Provider) {
	name = strings.ToLower(name)
	providersMu.Lock()
	defer providersMu.Unlock()
	providers[name] = provider
}

// ProviderForName looks up a provider by name.
func ProviderForName(name string) (Provider, bool) {
	name = strings.ToLower(name)
	providersMu.RLock()
	defer providersMu.RUnlock()
	p, ok := providers[name]
	return p, ok
}
