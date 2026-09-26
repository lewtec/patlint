package pattern

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNodeToPatSexp(t *testing.T) {
	cases := []struct {
		pat string
	}{
		{`(token "interface{}")`},
		{`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) "," (capture ERR any) ")")`},
		{`(seq (capture F (ref "go:strings::SplitN")) "(" (capture S any) "," (capture SEP any) "," "2" ")")`},
		{`(seq (unify a any) ">" (unify b any) ":" (unify a any) "?" (unify b any))`},
		{`(seq (assert_not_behind (seq (capture NAME (regex "^Err")) "=" (capture PKG any) ".")) (capture E (ref "go:errors::New")) "(" (* any) ")")`},
		{`(seq (token "if") (unify a any) (capture op (regex ">=?")) (unify b any) "{" (token "return") (unify a any) "}" (group (alt (seq (token "return") (unify b any)) (seq (token "else") "{" (token "return") (unify b any) "}"))))`},
		{`(seq (capture F (ref "go:fmt::Errorf")) "(" (* any) (capture E (ref "go:errors::New")) "(" (* any) ")" (* any) ")")`},
	}
	for _, tc := range cases {
		t.Run(tc.pat, func(t *testing.T) {
			n, err := ParsePattern(tc.pat)
			require.NoError(t, err,
				"parse: %v", err)

			core, err := NodeToPat(n)
			require.NoError(t, err,
				"NodeToPat: %v", err)
			{

				err := CheckPat(core)
				require.NoError(t, err,
					"CheckPat: %v", err)
			}

			legacy, err := PatToNode(core)
			require.NoError(t, err,
				"PatToNode: %v", err)
			{

				_, err := compileLegacyNode(legacy)
				require.NoError(t, err,
					"compileLegacyNode: %v", err)
			}
			{

				// Full compile path (bridge inside)
				_, err := compilePattern(n)
				require.NoError(t, err,
					"compilePattern: %v", err)
			}

		})
	}
}

func TestRepStarCaptureShape(t *testing.T) {
	n, err := ParsePattern(`(* (capture c (ref "go:errors::New")))`)
	require.NoError(t, err)

	core, err := NodeToPat(n)
	require.NoError(t, err)

	rep, ok := core.(Rep)
	require.True(t, ok,
		"want Rep, got %T", core)
	require.False(t, rep.Min != 0 || rep.Max >= 0,
		"want unbounded star, got min=%d max=%d", rep.Min, rep.Max)

	cap, ok := rep.Body.(Capture)
	require.False(t, !ok || cap.Name != "c",
		"want Capture c, got %#v", rep.Body)
	{

		_, ok := cap.Body.(Ref)
		require.True(t, ok,
			"want Ref body, got %T", cap.Body)
	}

}

func TestCheckPatRejectsMixedDiscipline(t *testing.T) {
	p := Seq{Items: []Pat{
		Capture{Name: "a", Body: Any{}},
		Unify{Name: "a", Body: Any{}},
	}}
	err := CheckPat(p)
	require.Error(t, err,
		"expected discipline error")

}

func TestCheckPatRejectsCaptureAroundRep(t *testing.T) {
	p := Capture{Name: "c", Body: Rep{Min: 0, Max: -1, Body: Any{}}}
	err := CheckPat(p)
	require.Error(t, err,
		"expected capture-around-rep error")

}
