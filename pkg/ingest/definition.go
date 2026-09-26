package ingest

import (
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/projectfs"
	"github.com/lewtec/patlint/pkg/store"
)

// Definition is a resolved atom location (UTF-8 half-open span).
// Shared by LSP textDocument/definition and the TUI editor (g d).
type Definition struct {
	Path      string // absolute when joinable from root
	StartByte uint32
	EndByte   uint32
	Reference string
}

// DefinitionQuery is the spine input for DefinitionAt.
type DefinitionQuery struct {
	Root    string
	FileRel string // project-relative path (optional ./)
	Text    string // full file text (IdentifierAt fallback)
	ByteOff int
	// Project is an optional graph (LSP last-good snapshot). When nil, DefinitionAt
	// seeds only the cursor file neighborhood (NavigateAround) — never a full project walk.
	Project *project.Result
	FS      projectfs.FS // nil → OS
}

// DefinitionFromResult finds the atom for ref in result and maps it to a Definition.
// Used by DefinitionAt and by LSP helpers that already navigated.
func DefinitionFromResult(root string, result *project.Result, ref Reference) (Definition, bool) {
	if result == nil {
		return Definition{}, false
	}
	ent, ref := findAtomForRef(result, ref)
	if ent == nil {
		return Definition{}, false
	}
	er := ParseReference(ent.Reference)
	abs := er.Path
	if abs != "" && !filepath.IsAbs(abs) {
		abs = lewpath.New(root, filepath.FromSlash(strings.TrimPrefix(er.Path, "./"))).String()
	}
	if abs == "" {
		return Definition{}, false
	}
	return Definition{
		Path:      abs,
		StartByte: ent.StartByte,
		EndByte:   ent.EndByte,
		Reference: ent.Reference,
	}, true
}

// findAtomForRef mirrors LSP definition entity lookup: exact, then same-path name, then unique name.
func findAtomForRef(result *project.Result, ref Reference) (*project.Atom, Reference) {
	canon := ref.String()
	for i := range result.Atoms {
		if result.Atoms[i].Reference == canon {
			return &result.Atoms[i], ref
		}
	}
	if ref.Name != "" {
		for i := range result.Atoms {
			er := ParseReference(result.Atoms[i].Reference)
			if er.Name == ref.Name && (ref.Path == "" || sameRelPath(ref, er)) {
				return &result.Atoms[i], er
			}
		}
		for i := range result.Atoms {
			er := ParseReference(result.Atoms[i].Reference)
			if er.Name == ref.Name {
				return &result.Atoms[i], er
			}
		}
	}
	return nil, ref
}

func sameRelPath(a, b Reference) bool {
	pa := strings.TrimPrefix(filepath.ToSlash(a.Path), "./")
	pb := strings.TrimPrefix(filepath.ToSlash(b.Path), "./")
	return pa == pb
}

// DefinitionFromStore is DefinitionFromResult on RelationAtom / RelationAtomInFile.
func DefinitionFromStore(root string, st *store.Store, ref Reference) (Definition, bool) {
	if st == nil {
		return Definition{}, false
	}
	view := store.Project(st, nil)
	return DefinitionFromResult(root, view, ref)
}
