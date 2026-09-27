package ingestutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDottedReceiver(t *testing.T) {
	_, ok := DottedReceiver("")
	require.False(t, ok, "empty")
	_, ok = DottedReceiver("plain")
	require.False(t, ok, "no dot")

	recv, ok := DottedReceiver("Class.method")
	require.True(t, ok)
	require.Equal(t, "Class", recv)

	recv, ok = DottedReceiver("Outer.Inner.m")
	require.True(t, ok)
	require.Equal(t, "Outer.Inner", recv)
}

func TestIdentUsedJava(t *testing.T) {
	require.True(t, IdentUsedJava("$foo bar", "$foo"),
		"want hit")
	require.False(t, IdentUsedJava("a$foo", "$foo"),
		"mid-ident")

}

func TestMissingSubstrings(t *testing.T) {
	got := MissingSubstrings("import A\n", []string{"import A", "import B", ""})
	require.Equal(t, []string{"import B"}, got)

}

func TestIsCStyleDocCommentLine(t *testing.T) {
	require.True(t, IsCStyleDocCommentLine("// x"))
	require.True(t, IsCStyleDocCommentLine("* x"))
	require.False(t, IsSlashSlashCommentLine("* x"))

}
