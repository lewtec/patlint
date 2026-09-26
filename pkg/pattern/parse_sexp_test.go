package pattern

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSexpRoundTripFormat(t *testing.T) {
	cases := []string{
		`(token "interface{}")`,
		`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) "," (capture ERR any) ")")`,
		`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG (regex "(?i)^failed to\\s+(.*)" 1)) "," (capture ERR any) ")")`,
		`(seq (capture blank (regex "^_$")) (capture op (regex "^(:=|=)$" 1)) (group (seq (capture f (regex "^[A-Za-z_]")) "(" (* any) ")")))`,
		`(* (unify c (ref "go:errors::New")))`,
		`(? (capture c (ref "go:context::Background")))`,
		`(seq (unify a any) ">" (unify b any) ":" (unify a any) "?" (unify b any))`,
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			p, err := ParseToPat(src)
			require.NoError(t, err)

			// ParseSexp runs Expand: numeric/?/+ rep macros expand. Round-trip is
			// stable after expand (Format → ParseSexp → Format), not identity with
			// pre-expand IR (e.g. LocalOptional → (alt (seq) body)).
			lisp := FormatSexp(p)
			p2, err := ParseSexp(lisp)
			require.NoError(t, err)

			p3, err := ParseSexp(FormatSexp(p2))
			require.NoError(t, err)
			require.Equal(t, FormatSexp(p2), FormatSexp(p3), "expand not stable")

		})
	}
}

func TestParseToPatLispString(t *testing.T) {
	p, err := ParseToPat(`(token "interface{}")`)
	require.NoError(t, err)

	tok, ok := p.(Token)
	require.True(t, ok, "%#v", p)
	require.Equal(t, "interface{}", tok.Text)

}

func TestParseToPatBareAtom(t *testing.T) {
	p, err := ParseToPat(`interface{}`)
	require.NoError(t, err)

	got, ok := p.(Lit)
	require.True(t, ok, "%#v", p)
	require.Equal(t, "interface{}", got.Text)

}
