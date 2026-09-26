// Package rust registers the Rust language family for refactree ingest and moves.
//
// Module model (v1):
//   - Each .rs file is a module (file-as-module lattice).
//   - crate:: / mod declarations resolve within the crate source root
//     (directory of lib.rs/main.rs, or the importer directory as fallback).
//   - External crates resolve to the rust: provider (no cargo registry crawl yet).
//
// Grains: atom, module, package — full mv surface for fuzzy iteration.
package rust

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
)

// FamilyID is the registry id for the Rust lattice.
const FamilyID = "rust"

// Family is the Rust family handle.
var Family = ingest.RegisterFamily(FamilyID, ingest.FamilySpec{
	Lattice:       lattice{},
	ResolveImport: ResolveImport,
})

type lattice struct{}

func (lattice) Grains() []ingest.MoveGrain {
	return []ingest.MoveGrain{ingest.MoveGrainAtom, ingest.MoveGrainModule, ingest.MoveGrainPackage}
}

func (lattice) ModuleKey(filePath string) string { return ingest.FileModuleKey(filePath) }

func (m lattice) SameModule(a, b string) bool { return m.ModuleKey(a) == m.ModuleKey(b) }

func (lattice) ListNodes(result *project.Result, grain ingest.MoveGrain, projectFamily string) []ingest.MoveNode {
	switch grain {
	case ingest.MoveGrainAtom:
		return ingest.ListAtomMoveNodes(result, projectFamily, skipRustAtom)
	case ingest.MoveGrainModule:
		return ingest.ListModuleFileMoveNodes(result, projectFamily)
	case ingest.MoveGrainPackage:
		return ingest.ListPackageMoveNodes(result, projectFamily, skipRustPackageDir)
	default:
		return nil
	}
}

func skipRustAtom(name string) bool {
	// main is the binary entry; moving it is almost never useful for fuzzy.
	leaf := ingest.AtomName(name)
	return leaf == "main"
}

func skipRustPackageDir(dir string) bool {
	dir = strings.Trim(strings.TrimPrefix(dir, "./"), "/")
	// Avoid relocating cargo layout roots wholesale as "packages".
	switch dir {
	case "src", "tests", "benches", "examples", "target":
		return true
	}
	return strings.HasPrefix(dir, "target/")
}
