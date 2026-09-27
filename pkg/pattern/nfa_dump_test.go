package pattern

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatNFAFromPat_Lit(t *testing.T) {
	p, err := ParseToPat(`(token "interface{}")`)
	require.NoError(t, err)

	s, err := FormatNFAFromPat(p)
	require.NoError(t, err)
	require.Contains(t, s, "START")
	require.Contains(t, s, "ACCEPT")
	require.Contains(t, s, `token "interface{}"`)

}

func TestFormatCompiledPlan_Under(t *testing.T) {
	m, err := ParseMatcher(`(under (token "func") (token "x"))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	s := FormatCompiledPlan(cm)
	require.Contains(t, s, "under")
	require.Contains(t, s, "region:")
	require.Contains(t, s, "body:")
	require.Contains(t, s, "ε-NFA")

}

func TestFormatNFADot(t *testing.T) {
	p := Lit{Text: "if"}
	dot, err := FormatNFADot(p)
	require.NoError(t, err)
	require.Contains(t, dot, "digraph")
	require.Contains(t, dot, "lit")

}
