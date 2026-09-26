package ingest

import (
	"github.com/lewtec/patlint/pkg/project"
	// MoveGrain is a level in the family module lattice (fuzzy / move planning).
)

type MoveGrain string

const (
	MoveGrainAtom    MoveGrain = "atom"
	MoveGrainModule  MoveGrain = "module"
	MoveGrainPackage MoveGrain = "package"
)

// MoveNode is an enumerable source at a grain for lattice-aware tooling.
type MoveNode struct {
	Grain MoveGrain
	// Reference is a full path reference (with symbol for atom grain).
	Reference string
	// Path is the slash path with leading ./ (file or directory).
	Path string
	// Name is non-empty for atom grain.
	Name string
}

// MoveLattice describes grains and module boundaries for one language family.
// Registered on FamilySpec; fuzzy and planners read it from the family registry.
type MoveLattice interface {
	Grains() []MoveGrain
	// ListNodes enumerates sources; projectFamily is the catalog family id.
	ListNodes(result *project.Result, grain MoveGrain, projectFamily string) []MoveNode
	// SameModule reports whether two paths (./rel) share a module boundary.
	SameModule(pathA, pathB string) bool
	// ModuleKey returns a stable module identity for a file path (./rel).
	ModuleKey(filePath string) string
}
