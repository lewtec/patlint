package reference

import "testing"

func TestSortScopeChildrenByPath(t *testing.T) {
	in := []ScopeChild{
		{Ref: Reference{Path: "b"}},
		{Ref: Reference{Path: "a"}},
	}
	SortScopeChildrenByPath(in)
	if in[0].Ref.Path != "a" || in[1].Ref.Path != "b" {
		t.Fatalf("%v", in)
	}
}

func TestSortScopeChildrenByKindThenPath(t *testing.T) {
	in := []ScopeChild{
		{Kind: ScopeChildFile, Ref: Reference{Path: "b"}},
		{Kind: ScopeChildDir, Ref: Reference{Path: "z"}},
		{Kind: ScopeChildFile, Ref: Reference{Path: "a"}},
	}
	SortScopeChildrenByKindThenPath(in)
	if in[0].Kind != ScopeChildDir || in[1].Ref.Path != "a" || in[2].Ref.Path != "b" {
		t.Fatalf("%+v", in)
	}
}
