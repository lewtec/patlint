package ingest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQualifierBefore(t *testing.T) {
	src := []byte("pkga.Helper()\nmedia.PlaybackStatus\nHelper()\n")
	qs, qe, ok := qualifierBefore(src, uint32(5))
	require.True(t, ok)
	require.Equal(t, "pkga", string(src[qs:qe]))

	at := uint32(len("pkga.Helper()\nmedia."))
	qs, qe, ok = qualifierBefore(src, at)
	require.True(t, ok)
	require.Equal(t, "media", string(src[qs:qe]))

	bare := uint32(len("pkga.Helper()\nmedia.PlaybackStatus\n"))
	_, _, ok = qualifierBefore(src, bare)
	require.False(t, ok, "bare Helper has no qualifier")

	span := []byte("pkga.Helper")
	qs, qe, ok = qualifierInSpan(span)
	require.True(t, ok)
	require.Equal(t, "pkga", string(span[qs:qe]))
}
