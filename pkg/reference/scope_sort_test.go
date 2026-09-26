package reference

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortScopeChildrenByPath(t *testing.T) {
	in := []ScopeChild{
		{Ref: Reference{Path: "b"}},
		{Ref: Reference{Path: "a"}},
	}
	SortScopeChildrenByPath(in)
	require.Equal(t, "a", in[0].Ref.Path)
	require.Equal(t, "b", in[1].Ref.Path)

}

func TestSortScopeChildrenByKindThenPath(t *testing.T) {
	in := []ScopeChild{
		{Kind: ScopeChildFile, Ref: Reference{Path: "b"}},
		{Kind: ScopeChildDir, Ref: Reference{Path: "z"}},
		{Kind: ScopeChildFile, Ref: Reference{Path: "a"}},
	}
	SortScopeChildrenByKindThenPath(in)
	require.Equal(t, ScopeChildDir, in[0].Kind)
	require.Equal(t, "a", in[1].Ref.Path)
	require.Equal(t, "b", in[2].Ref.Path)

}
