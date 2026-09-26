package datalog

import "github.com/lewtec/patlint/pkg/store"

// MoveProgram is stratum 3 cross-file mv: hole, place, marked import rewrite.
func MoveProgram() Program {
	return Program{
		cl(l(store.RelationEdit, "?f", "?s", "?e", ""),
			l("$move_hole", "?f", "?s", "?e")),

		cl(l(store.RelationEdit, "?f", "?s", "?e", "?t"),
			l("$move_place", "?f", "?s", "?e", "?t")),

		cl(l(store.RelationEdit, "?f", "?ps", "?pe", "?neu"),
			l(store.RelationAlias, "?aref", "?s", "?e", "?target", "?spec", "?ps", "?pe"),
			l("$ref_file", "?aref", "?f"),
			l("$rewrite_spec", "?f", "?spec", "?neu"),
			l("$span_ok", "?ps", "?pe"),
			l("$not_move_file", "?f")),
	}
}
