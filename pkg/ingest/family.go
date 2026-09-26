package ingest

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Family is a registered language-family handle. Create with RegisterFamily.
// Language ids live on pack as-family claims, not on this registry.
type Family struct {
	id      string
	lattice MoveLattice
	resolve func(spec string, ctx ImportResolveContext) string
}

// FamilySpec configures a family at registration time.
type FamilySpec struct {
	// Lattice is the higher-level move/module model (optional; nil = no mv lattice).
	Lattice MoveLattice
	// ResolveImport maps an import spec to a product ref (go.mod, node, …).
	ResolveImport func(spec string, ctx ImportResolveContext) string
}

var (
	familyMu   sync.RWMutex
	familyByID = map[string]*Family{}
)

// RegisterFamily registers a family. Panics on empty id, duplicate id, or nil
// family after conflict checks.
func RegisterFamily(id string, spec FamilySpec) *Family {
	id = strings.TrimSpace(id)
	if id == "" {
		panic("ingest: RegisterFamily with empty id")
	}

	familyMu.Lock()
	defer familyMu.Unlock()

	if prev, ok := familyByID[id]; ok {
		// Idempotent only when lattice identity matches (same pointer or both nil).
		if (prev.lattice == nil && spec.Lattice == nil) || prev.lattice == spec.Lattice {
			if spec.ResolveImport != nil && prev.resolve == nil {
				prev.resolve = spec.ResolveImport
			}
			return prev
		}
		panic(fmt.Sprintf("ingest: family %q already registered with a different lattice", id))
	}
	f := &Family{id: id, lattice: spec.Lattice, resolve: spec.ResolveImport}
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

// ID returns the registry family id (opaque registry string).
func (f *Family) ID() string {
	if f == nil {
		return ""
	}
	return f.id
}

// Lattice returns the family's move lattice, or nil.
func (f *Family) Lattice() MoveLattice {
	if f == nil {
		return nil
	}
	return f.lattice
}

// FamilyByID looks up a registered family handle.
func FamilyByID(id string) (*Family, bool) {
	familyMu.RLock()
	defer familyMu.RUnlock()
	f, ok := familyByID[id]
	return f, ok
}

// LatticeForFamily returns the move lattice registered for family id.
func LatticeForFamily(familyID string) (MoveLattice, bool) {
	f, ok := FamilyByID(familyID)
	if !ok || f.lattice == nil {
		return nil, false
	}
	return f.lattice, true
}

// IsKnownFamily reports whether family id was registered via RegisterFamily.
func IsKnownFamily(family string) bool {
	familyMu.RLock()
	defer familyMu.RUnlock()
	_, ok := familyByID[family]
	return ok
}

// Families returns registered family ids in sorted order.
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
