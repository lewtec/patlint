package pattern

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatSexpBasic(t *testing.T) {
	p, err := ParseToPat(`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) "," (capture ERR any) ")")`)
	require.NoError(t, err)

	s := FormatSexp(p)
	require.Contains(t, s, "(capture F")
	require.Contains(t, s, `(ref "go:fmt::Errorf")`)

	js, err := FormatSexpJSON(p)
	require.NoError(t, err)
	require.Contains(t, js, `"capture"`)
	require.Contains(t, js, `"go:fmt::Errorf"`)
}

func TestFormatSexpRepUnify(t *testing.T) {
	p, err := ParseToPat(`(* (unify c (ref "go:errors::New")))`)
	require.NoError(t, err)

	s := FormatSexp(p)
	require.Contains(t, s, "(* ")
	require.Contains(t, s, "(unify c")
}

func TestFormatSexpOptional(t *testing.T) {
	p, err := ParseToPat(`(? (capture c (ref "go:context::Background")))`)
	require.NoError(t, err)

	s := FormatSexp(p)
	require.True(t, strings.Contains(s, "(alt ") || strings.Contains(s, "(? "), "sexp=%s", s)
}
