// Package ecma registers the ECMA family (file-as-module import/export lattice).
package ecma

import (
	"github.com/lewtec/patlint/pkg/ingest"

	"github.com/lewtec/patlint/pkg/project"
)

// FamilyID is the registry id for the ECMA lattice.
const FamilyID = "ecma"

// Family is the ECMA family handle (javascript, typescript, tsx, svelte, vue, astro, …).
var Family = ingest.RegisterFamily(FamilyID, ingest.FamilySpec{Lattice: lattice{}})

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
