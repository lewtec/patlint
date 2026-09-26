package store

import (
	"sort"
	"strconv"
	"strings"
)

// Closed relation names (SPEC Graph engine).
const (
	RelationFile         = "file"
	RelationAtom         = "atom"
	RelationImport       = "import"
	RelationUse          = "use"
	RelationUseSegment       = "use_seg"
	RelationScope        = "scope"
	RelationFlow         = "flow"
	RelationPackage      = "package"
	RelationReexport     = "reexport"
	RelationDefault      = "default_export"
	RelationLanguage         = "lang"
	RelationDeclares     = "declares"
	RelationAlias        = "alias"
	RelationBinds        = "binds"
	RelationImportTarget = "import_target"
	RelationStarBase     = "star_base"
	RelationVisible      = "visible"
	RelationFinding      = "finding"
	RelationEdit         = "edit"
	RelationUsedName     = "used_name"
	RelationResolvedSpecifier = "resolved_spec"
	RelationAtomInFile   = "atom_in_file"
	RelationBound        = "bound"
)

// Tuple is one row. All fields are strings (bytes as decimal).
type Tuple []string

type relation struct {
	byKey map[string]int
	rows  []Tuple
}

// Store is a set of named relations.
type Store struct {
	rels map[string]*relation
}

// New is an empty store.
func New() *Store {
	return &Store{rels: map[string]*relation{}}
}

func keyOf(t Tuple) string {
	return strings.Join(t, "\x00")
}

// Insert adds t to rel. Duplicate rows are ignored.
func (s *Store) Insert(rel string, t Tuple) bool {
	if s == nil {
		return false
	}
	r := s.rels[rel]
	if r == nil {
		r = &relation{byKey: map[string]int{}}
		s.rels[rel] = r
	}
	k := keyOf(t)
	if _, ok := r.byKey[k]; ok {
		return false
	}
	row := make(Tuple, len(t))
	copy(row, t)
	r.byKey[k] = len(r.rows)
	r.rows = append(r.rows, row)
	return true
}

// Reset drops rel.
func (s *Store) Reset(rel string) {
	if s == nil {
		return
	}
	delete(s.rels, rel)
}

// ResetFile drops rows of rel whose first column is file.
func (s *Store) ResetFile(rel, file string) {
	if s == nil || file == "" {
		return
	}
	old := s.Rows(rel)
	s.Reset(rel)
	for _, t := range old {
		if len(t) == 0 || samePath(t[0], file) {
			continue
		}
		if rel == RelationUse || rel == RelationUseSegment {
			s.Append(rel, t)
		} else {
			s.Insert(rel, t)
		}
	}
}

// Append adds t even when a duplicate key exists (ingest uses).
func (s *Store) Append(rel string, t Tuple) {
	if s == nil {
		return
	}
	r := s.rels[rel]
	if r == nil {
		r = &relation{byKey: map[string]int{}}
		s.rels[rel] = r
	}
	row := make(Tuple, len(t))
	copy(row, t)
	r.rows = append(r.rows, row)
}

// Contains reports whether rel holds t.
func (s *Store) Contains(rel string, t Tuple) bool {
	if s == nil {
		return false
	}
	r := s.rels[rel]
	if r == nil {
		return false
	}
	_, ok := r.byKey[keyOf(t)]
	return ok
}

// Rows is the live slice for rel. Do not append to it.
func (s *Store) Rows(rel string) []Tuple {
	if s == nil {
		return nil
	}
	r := s.rels[rel]
	if r == nil {
		return nil
	}
	return r.rows
}

// RelationNames lists relations that have at least one row.
func (s *Store) RelationNames() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.rels))
	for n, r := range s.rels {
		if r != nil && len(r.rows) > 0 {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// Itoa is a decimal uint32 field.
func Itoa(n uint32) string { return strconv.FormatUint(uint64(n), 10) }

// Atoi parses a decimal field. Invalid → 0.
func Atoi(s string) uint32 {
	n, _ := strconv.ParseUint(s, 10, 32)
	return uint32(n)
}

// ItoaInt is a signed decimal field.
func ItoaInt(n int) string { return strconv.Itoa(n) }

// AtoiInt parses a signed decimal field.
func AtoiInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
