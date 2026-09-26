// Package c registers the C-family languages (C and C++) for refactree ingest.
//
// Module model (v1): each translation unit / header file is a module.
// #include "…" is a star-import of the included file's top-level atoms.
// System includes (#include <…>) resolve to the c: provider (no deep expand).
// No MoveDriver yet — rename/move waits on fuzzy corpora.
package c

import (
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
)

// FamilyID is the registry id for the C/C++ file-as-module lattice.
const FamilyID = "c"

// Family is the C-family handle. Surfaces: c, cpp.
var Family = ingest.RegisterFamily(FamilyID, ingest.FamilySpec{
	Lattice:       lattice{},
	ResolveImport: ResolveInclude,
})

type lattice struct{}

func (lattice) Grains() []ingest.MoveGrain {
	return []ingest.MoveGrain{ingest.MoveGrainAtom, ingest.MoveGrainModule}
}

func (lattice) ModuleKey(filePath string) string { return ingest.FileModuleKey(filePath) }

func (m lattice) SameModule(a, b string) bool { return m.ModuleKey(a) == m.ModuleKey(b) }

func (lattice) ListNodes(result *project.Result, grain ingest.MoveGrain, projectFamily string) []ingest.MoveNode {
	switch grain {
	case ingest.MoveGrainAtom:
		return ingest.ListAtomMoveNodes(result, projectFamily, nil)
	case ingest.MoveGrainModule:
		return ingest.ListModuleFileMoveNodes(result, projectFamily)
	default:
		return nil
	}
}
