package ingest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemberReceiver(t *testing.T) {
	cases := map[string]string{
		"Session.Close":    "Session",
		"*Session.Close":   "Session",
		"Set[T].Add":       "Set",
		"*Set[T].Add":      "Set",
		"SudoCommand.Slug": "SudoCommand",
		"Helper":           "",
	}
	for name, want := range cases {
		got := memberReceiver(name)
		require.Equal(t, want, got, name)
	}
}

func TestIdentExported(t *testing.T) {
	require.True(t, identifierExported("Helper"))
	require.False(t, identifierExported("helper"))
	require.False(t, identifierExported("createWorkspacedShim"))
}

func TestTestFile(t *testing.T) {
	require.True(t, testFile("pkga/a_test.go"))
	require.False(t, testFile("pkga/a.go"))
	require.True(t, testFile("foo_test.py"))
}

func TestContainsIdent(t *testing.T) {
	require.True(t, containsIdentifier([]byte("Control PoolKind = iota\n"), "iota"),
		"want iota")
	require.False(t, containsIdentifier([]byte("iotaFoo"), "iota"),
		"prefix")

}
