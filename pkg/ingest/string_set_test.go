package ingest

import "testing"

func TestStringSet(t *testing.T) {
	s := NewStringSet("a", "b", "")
	if s.Len() != 2 || !s.Has("a") || s.Has("") {
		t.Fatalf("%v", s)
	}
	s.Add("c")
	if !s.Has("c") || s.Len() != 3 {
		t.Fatalf("%v", s)
	}
	s.Delete("a")
	if s.Has("a") || s.Len() != 2 {
		t.Fatalf("%v", s)
	}
}
