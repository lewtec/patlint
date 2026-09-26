package reference

import (
	"cmp"
	"slices"
	"strings"
)

// SortScopeChildrenByPath sorts children by Ref.Path ascending.
func SortScopeChildrenByPath(children []ScopeChild) {
	slices.SortFunc(children, func(a, b ScopeChild) int {
		return strings.Compare(a.Ref.Path, b.Ref.Path)
	})
}

// SortScopeChildrenByKindThenPath sorts by Kind, then Ref.Path.
func SortScopeChildrenByKindThenPath(children []ScopeChild) {
	slices.SortFunc(children, func(a, b ScopeChild) int {
		if c := cmp.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		return strings.Compare(a.Ref.Path, b.Ref.Path)
	})
}
