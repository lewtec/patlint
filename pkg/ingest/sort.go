package ingest

import (
	"cmp"
	"github.com/lewtec/patlint/pkg/project"
	"slices"
)

// SortResult orders Files, Atoms, Aliases, and Uses for stable compare.
func SortResult(r *project.Result) {
	if r == nil {
		return
	}
	slices.SortFunc(r.Files, func(a, b project.File) int {
		return cmp.Compare(a.Path, b.Path)
	})
	slices.SortFunc(r.Atoms, func(a, b project.Atom) int {
		return cmp.Or(
			cmp.Compare(a.Reference, b.Reference),
			cmp.Compare(a.StartByte, b.StartByte),
		)
	})
	slices.SortFunc(r.Aliases, func(a, b project.Alias) int {
		return cmp.Or(
			cmp.Compare(a.Reference, b.Reference),
			cmp.Compare(a.StartByte, b.StartByte),
		)
	})
	slices.SortFunc(r.Uses, func(a, b project.Use) int {
		return cmp.Or(
			cmp.Compare(a.Reference, b.Reference),
			cmp.Compare(a.StartByte, b.StartByte),
		)
	})
}
