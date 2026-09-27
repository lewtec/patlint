package datalog

import "github.com/lewtec/patlint/pkg/store"

func l(rel string, args ...string) Lit {
	as := make([]Arg, len(args))
	for i, a := range args {
		if len(a) > 0 && a[0] == '?' {
			as[i] = Var(a[1:])
		} else {
			as[i] = Const(a)
		}
	}
	return Lit{Rel: rel, Args: as}
}

func cl(head Lit, body ...Lit) Clause { return Clause{Head: head, Body: body} }

// ResolveProgram is prelude stratum 2: alias + binds.
func ResolveProgram() Program {
	imp := []string{"?f", "?local", "?spec", "?mem", "?s", "?e", "?ts", "?te", "?al", "?ps", "?pe", "?star"}
	return Program{
		cl(l(store.RelationVisible, "?f", "?s", "?leaf", "?r"),
			l(store.RelationDeclares, "?f", "?s", "?leaf", "?r")),

		cl(l(store.RelationVisible, "?f", "?s", "?leaf", "?r"),
			l(store.RelationScope, "?f", "?s", "?p", "?a", "?b", "?hole"),
			l("$ge0", "?p"),
			l(store.RelationVisible, "?f", "?p", "?leaf", "?r")),

		cl(l(store.RelationResolvedSpecifier, "?f", "?spec", "?base"),
			l(store.RelationImport, imp...),
			l("$mod_resolve", "?spec", "?f", "?base")),

		cl(l(store.RelationResolvedSpecifier, "?f", "?spec", "?base"),
			l(store.RelationReexport, "?f", "?ex", "?sn", "?spec", "?star", "?ss", "?se"),
			l("$mod_resolve", "?spec", "?f", "?base")),

		cl(l(store.RelationStarBase, "?f", "?base"),
			l(store.RelationImport, "?f", "?local", "?spec", "?mem", "?s", "?e", "?ts", "?te", "?al", "?ps", "?pe", "1"),
			l(store.RelationResolvedSpecifier, "?f", "?spec", "?base")),

		cl(l(store.RelationImportTarget, "?f", "?local", "?target", "?mem", "?al"),
			l(store.RelationImport, "?f", "?local", "?spec", "?mem", "?s", "?e", "?ts", "?te", "?al", "?ps", "?pe", "0"),
			l(store.RelationResolvedSpecifier, "?f", "?spec", "?base"),
			l("$import_member", "?base", "?mem", "?target")),

		// alias: file ref + chosen span + target + import path token
		cl(l(store.RelationAlias, "?aref", "?as", "?ae", "?target", "?spec", "?ps", "?pe"),
			l(store.RelationImport, "?f", "?local", "?spec", "?mem", "?s", "?e", "?ts", "?te", "?al", "?ps", "?pe", "0"),
			l(store.RelationImportTarget, "?f", "?local", "?target", "?mem", "?al"),
			l("$file_ref", "?f", "?aref"),
			l("$pick_span", "?s", "?e", "?ts", "?te", "?as", "?ae")),

		// star reexport: file → source file
		cl(l(store.RelationAlias, "?aref", "0", "0", "?tref", "", "0", "0"),
			l(store.RelationReexport, "?f", "?ex", "?sn", "?spec", "1", "?ss", "?se"),
			l(store.RelationResolvedSpecifier, "?f", "?spec", "?base"),
			l("$file_ref", "?f", "?aref"),
			l("$file_ref_of", "?base", "?tref")),

		// named reexport
		cl(l(store.RelationAlias, "?eref", "?ss", "?se", "?target", "", "0", "0"),
			l(store.RelationReexport, "?f", "?ex", "?sn", "?spec", "0", "?ss", "?se"),
			l(store.RelationResolvedSpecifier, "?f", "?spec", "?base"),
			l("$reexport_names", "?ex", "?sn", "?en", "?srcn"),
			l("$atom_ref", "?f", "?en", "?eref"),
			l("$reexport_target", "?base", "?srcn", "?target")),

		// default export
		cl(l(store.RelationAlias, "?aref", "0", "0", "?tref", "", "0", "0"),
			l(store.RelationDefault, "?f", "?name"),
			l("$file_ref", "?f", "?aref"),
			l("$atom_ref", "?f", "?name", "?tref")),
	}
}
