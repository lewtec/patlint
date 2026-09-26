package pattern

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCaptureNames(t *testing.T) {
	pat, err := ParsePattern(`(seq (token "func") (capture name (regex "^Test(?P<rest>.*)")) "(" (capture t any) (* any) (ref "go:testing::T") ")")`)
	require.NoError(t, err)

	names := CaptureNames(pat)
	want := []string{"name", "rest", "t"}
	require.Len(t, names, len(want),
		"got %v want %v", names, want)

	for i := range want {
		require.Equal(t, want[i], names[i],
			"got %v want %v", names, want)

	}
}
