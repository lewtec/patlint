package pattern

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoercePatternStringSexp(t *testing.T) {
	p, err := CoercePattern(`(token "interface{}")`, CoerceOptions{})
	require.NoError(t, err)

	_, ok := p.(Token)
	require.True(t, ok, "%#v", p)
}

func TestCoercePatternArraySexp(t *testing.T) {
	p, err := CoercePattern([]any{"token", "interface{}"}, CoerceOptions{})
	require.NoError(t, err)
	tok, ok := p.(Token)
	require.True(t, ok, "%#v", p)
	require.Equal(t, "interface{}", tok.Text)
}
