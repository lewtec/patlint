// Package zig registers the Zig language family for refactree ingest and moves.
//
// Module model (v1): each .zig file is a module (file-as-module lattice).
// @import("./x.zig") resolves to path: relative to the importer; bare std → zig:std.
// Extract covers functions, const/var, struct/enum/union members, and usages.
// Thin MoveDriver: top-level ExtractDecl/InsertDecl + @import path rewrites.
package zig

import (
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
)

// FamilyID is the registry id for the Zig lattice.
const FamilyID = "zig"

// Family is the Zig family handle.
var Family = ingest.RegisterFamily(FamilyID, ingest.FamilySpec{
	Lattice:       lattice{},
	ResolveImport: ResolveImport,
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
