package ingest

import (
	"slices"
	"strings"
	"sync"
)

// Family is a registered import resolver for one language id.
// Create with RegisterFamily. The id matches a pack as-family claim.
type Family struct {
	id      string
	resolve func(spec string, ctx ImportResolveContext) string
}

// FamilySpec configures a family at registration time.
type FamilySpec struct {
	// ResolveImport maps an import spec to a product ref.
	ResolveImport func(spec string, ctx ImportResolveContext) string
}

var (
	familyMu   sync.RWMutex
	familyByID = map[string]*Family{}
)

// RegisterFamily registers a family. Panics on an empty id.
// A second registration of the same id keeps the first resolver unless it was nil.
func RegisterFamily(id string, spec FamilySpec) *Family {
	id = strings.TrimSpace(id)
	if id == "" {
		panic("ingest: RegisterFamily with empty id")
	}

	familyMu.Lock()
	defer familyMu.Unlock()

	if prev, ok := familyByID[id]; ok {
		if spec.ResolveImport != nil && prev.resolve == nil {
			prev.resolve = spec.ResolveImport
		}
		return prev
	}
	f := &Family{id: id, resolve: spec.ResolveImport}
	familyByID[id] = f
	return f
}

// ResolveImport is the family's import-spec mapping, or "".
func (f *Family) ResolveImport(spec string, ctx ImportResolveContext) string {
	if f == nil || f.resolve == nil {
		return ""
	}
	return f.resolve(spec, ctx)
}

// ID returns the registry family id.
func (f *Family) ID() string {
	if f == nil {
		return ""
	}
	return f.id
}

// FamilyByID looks up a registered family handle.
func FamilyByID(id string) (*Family, bool) {
	familyMu.RLock()
	defer familyMu.RUnlock()
	f, ok := familyByID[id]
	return f, ok
}

// IsKnownFamily reports whether family id was registered via RegisterFamily.
func IsKnownFamily(family string) bool {
	familyMu.RLock()
	defer familyMu.RUnlock()
	_, ok := familyByID[strings.TrimSpace(family)]
	return ok
}

// Families returns registered family ids in stable order.
func Families() []string {
	familyMu.RLock()
	defer familyMu.RUnlock()
	out := make([]string, 0, len(familyByID))
	for id := range familyByID {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
