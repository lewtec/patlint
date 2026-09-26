package ingestutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaskNonNewlinesInPlace(t *testing.T) {
	buf := []byte("ab\ncd\nef")
	MaskNonNewlinesInPlace(buf, 0, 5)
	{
		got := string(buf)
		require.Equal(t, "  \n  \nef", got,
			"mask mid: got %q", got)
	}

	buf = []byte("xy")
	MaskNonNewlinesInPlace(buf, -1, 99)
	{
		got := string(buf)
		require.Equal(t, "  ", got,
			"mask clamp: got %q", got)
	}

	buf = []byte("keep")
	MaskNonNewlinesInPlace(buf, 2, 2)
	got := string(buf)
	require.Equal(t, "keep", got,
		"empty range: got %q", got)

}

func TestIdentUsed(t *testing.T) {
	require.True(t, IdentUsed("foo bar", "foo", IsIdentChar),
		"expected foo hit")
	require.False(t, IdentUsed("foobar", "foo", IsIdentChar),
		"prefix must not match")
	require.False(t, IdentUsed("xfoo", "foo", IsIdentChar),
		"suffix must not match")
	require.False(t, IdentUsed("", "foo", IsIdentChar),
		"empty text")
	require.False(t, IdentUsed("foo", "", IsIdentChar),
		"empty ident")
	require.False(t, IdentUsed("foo", "foo", nil),
		"nil isIdent")
	require.True(t, IdentUsed("$foo bar", "$foo", IsIdentCharJava),
		"expected $foo hit")
	require.False(t, IdentUsed("a$foo", "$foo", IsIdentCharJava),
		"$ mid-ident must not match as start")

}
