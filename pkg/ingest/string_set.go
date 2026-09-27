package ingest

// StringSet is an idiomatic string set (map[string]struct{}).
// The Go standard library has no set type (see package maps for map helpers only).
type StringSet map[string]struct{}

// NewStringSet returns a set containing vals (empty vals → empty non-nil set).
func NewStringSet(vals ...string) StringSet {
	s := make(StringSet, len(vals))
	for _, v := range vals {
		if v != "" {
			s[v] = struct{}{}
		}
	}
	return s
}

// Has reports membership.
func (s StringSet) Has(k string) bool {
	if s == nil {
		return false
	}
	_, ok := s[k]
	return ok
}

// Add inserts k. No-op for empty k or nil set (call NewStringSet first).
func (s StringSet) Add(k string) {
	if s == nil || k == "" {
		return
	}
	s[k] = struct{}{}
}

// Delete removes k if present.
func (s StringSet) Delete(k string) {
	if s == nil {
		return
	}
	delete(s, k)
}

// Len returns the number of elements.
func (s StringSet) Len() int {
	return len(s)
}
