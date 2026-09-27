package datalog

import "github.com/lewtec/patlint/pkg/store"

// Query ids written by QueryProgram (stratum 3).
const (
	DeadImportID    = "imports/unused-named"
	DeadImportLevel = "warning"
	DeadImportMsg   = "Unused named import"
)

// QueryProgram is stratum 3: finding / edit. dead-imports is negation over
// named import locals vs uses outside the import span.
func QueryProgram() Program {
	imp := []string{"?f", "?local", "?spec", "?mem", "?s", "?e", "?ts", "?te", "?al", "?ps", "?pe", "0"}
	use := []string{"?f", "?name", "?us", "?ue", "?sc", "?si", "?np", "?id"}
	return Program{
		cl(l(store.RelationUsedName, "?f", "?name"),
			l(store.RelationUse, use...),
			l("$outside_import", "?f", "?us", "?ue")),

		cl(l(store.RelationUsedName, "?f", "?name"),
			l(store.RelationUseSegment, "?f", "?id", "?idx", "?name", "?us", "?ue"),
			l("$outside_import", "?f", "?us", "?ue")),

		cl(l(store.RelationFinding, "?f", "?as", "?ae", DeadImportID, DeadImportLevel, DeadImportMsg),
			l(store.RelationImport, imp...),
			l("$import_name", "?local", "?spec", "?name"),
			l("$is_named", "?name"),
			Lit{Rel: store.RelationUsedName, Args: []Arg{Var("f"), Var("name")}, Neg: true},
			l("$import_span", "?s", "?e", "?ps", "?pe", "?as", "?ae")),

		cl(l(store.RelationEdit, "?f", "?as", "?ae", ""),
			l(store.RelationFinding, "?f", "?as", "?ae", DeadImportID, DeadImportLevel, DeadImportMsg)),
	}
}
