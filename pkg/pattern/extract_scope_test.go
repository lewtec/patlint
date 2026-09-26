package pattern

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"
)

func TestNestAndAssignScopes(t *testing.T) {
	fe := &project.FileExtract{
		Scopes: []project.ScopeDef{
			{StartByte: 0, EndByte: 20, Parent: 99}, // file-ish outer
			{StartByte: 5, EndByte: 15, Parent: 99}, // mid
			{StartByte: 7, EndByte: 10, Parent: 99}, // inner
			{StartByte: 5, EndByte: 15, Parent: 99}, // dup of mid
		},
		Atoms: []project.AtomDef{
			{Name: "file", StartByte: 1, EndByte: 2},
			{Name: "mid", StartByte: 6, EndByte: 7},
			{Name: "in", StartByte: 8, EndByte: 9},
		},
		Usages: []project.UsageDef{
			{Name: "u", StartByte: 8, EndByte: 9},
		},
	}
	nestAndAssignScopes(fe)
	require.Len(t, fe.Scopes, 3,
		"scopes=%d after dedup", len(fe.Scopes))
	require.Equal(t, -1, fe.Scopes[0].Parent,
		"outer parent=%d", fe.Scopes[0].Parent)
	require.Equal(t, 0, fe.Scopes[1].Parent,
		"mid parent=%d want 0", fe.Scopes[1].Parent)
	require.Equal(t, 1, fe.Scopes[2].Parent,
		"inner parent=%d want 1", fe.Scopes[2].Parent)
	require.Equal(t, 0, fe.Atoms[0].ScopeIdx,
		"file atom idx=%d want 0", fe.Atoms[0].ScopeIdx)
	require.Equal(t, 1, fe.Atoms[1].ScopeIdx,
		"mid atom idx=%d want 1", fe.Atoms[1].ScopeIdx)
	require.Equal(t, 2, fe.Atoms[2].ScopeIdx,
		"inner atom idx=%d want 2", fe.Atoms[2].ScopeIdx)
	require.Equal(t, 2, fe.Usages[0].ScopeIdx,
		"use idx=%d want 2", fe.Usages[0].ScopeIdx)

}

func TestInnermostScopeFileRoot(t *testing.T) {
	{
		got := innermostScope(nil, 0, 1)
		require.Equal(t, -1, got,
			"empty scopes idx=%d", got)
	}

	scopes := []project.ScopeDef{{StartByte: 10, EndByte: 20, Parent: -1}}
	got := innermostScope(scopes, 0, 1)
	require.Equal(t, -1, got,
		"uncovered idx=%d", got)

}
