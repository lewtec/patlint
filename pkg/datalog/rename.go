package datalog

import "github.com/lewtec/patlint/pkg/store"

// RenameProgram is stratum 3 identity rename: atom / binds / alias → edit.
func RenameProgram() Program {
	return Program{
		cl(l(store.RelationEdit, "?f", "?s", "?e", "?new"),
			l(store.RelationAtom, "?f", "?name", "?s", "?e", "?exp", "?si"),
			l(store.RelationAtomInFile, "?f", "?name", "?ref"),
			l("$rename_leaf", "?ref", "?new"),
			l("$span_ok", "?s", "?e")),

		cl(l(store.RelationEdit, "?f", "?s", "?e", "?new"),
			l(store.RelationBinds, "?f", "?s", "?e", "?from", "?target", "0"),
			l("$rename_leaf", "?target", "?new"),
			l("$span_ok", "?s", "?e")),

		cl(l(store.RelationEdit, "?f", "?s", "?e", "?new"),
			l(store.RelationAlias, "?aref", "?s", "?e", "?target", "?spec", "?ps", "?pe"),
			l("$ref_file", "?aref", "?f"),
			l("$rename_leaf", "?target", "?new"),
			l("$span_ok", "?s", "?e")),
	}
}
