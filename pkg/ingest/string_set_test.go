package ingest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringSet(t *testing.T) {
	s := NewStringSet("a", "b", "")
	require.Equal(t, 2, s.Len())
	require.True(t, s.Has("a"))
	require.False(t, s.Has(""))

	s.Add("c")
	require.True(t, s.Has("c"))
	require.Equal(t, 3, s.Len())

	s.Delete("a")
	require.False(t, s.Has("a"))
	require.Equal(t, 2, s.Len())
}
