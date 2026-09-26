package pattern

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodePatRoundTripCLI(t *testing.T) {
	cases := []string{
		`(token "interface{}")`,
		`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) "," (capture ERR any) ")")`,
		`(seq (capture F (ref "go:strings::SplitN")) "(" (capture S any) "," (capture SEP any) "," "2" ")")`,
		`(* (unify c (ref "go:errors::New")))`,
		`(? (capture c (ref "go:context::Background")))`,
		`(seq (unify a any) ">" (unify b any) ":" (unify a any) "?" (unify b any))`,
	}
	for _, pat := range cases {
		t.Run(pat, func(t *testing.T) {
			p, err := ParseToPat(pat)
			require.NoError(t, err)

			raw, err := json.Marshal(patToJSON(p))
			require.NoError(t, err)

			p2, err := DecodePatJSON(raw)
			require.NoError(t, err,
				"decode %s: %v", raw, err)

			s1, s2 := FormatSexp(p), FormatSexp(p2)
			require.Equal(t, s2, s1,
				"round-trip\n  got  %s\n  want %s\n  json %s", s2, s1, raw)

		})
	}
}

func TestDecodePatSexpFixtureShape(t *testing.T) {
	raw := []byte(`["token","interface{}"]`)
	p, err := DecodePatJSON(raw)
	require.NoError(t, err)

	tok, ok := p.(Token)
	require.False(t, !ok || tok.Text != "interface{}",
		"got %#v", p)

	n, err := PatToNode(p)
	require.NoError(t, err)
	require.False(t, n.Kind != "token" || n.Text != "interface{}",
		"node %+v", n)

}

func TestDecodePatRejectsCaptureAroundRep(t *testing.T) {
	// ["capture","c",["*",["any"]]] is rejected by CheckPat
	_, err := DecodePatJSON([]byte(`["capture","c",["*",["any"]]]`))
	require.Error(t, err,
		"want capture-around-rep error")

	msg := err.Error()
	require.True(t, strings.Contains(msg, "rep"),
		"want capture-around-rep error, got %v", err)

}

func TestResolvePatternIRFromSexp(t *testing.T) {
	op := Op{
		Mode:        "grep",
		PatternSexp: json.RawMessage(`["token","interface{}"]`),
	}
	err := op.ResolvePatternIR()
	require.NoError(t, err)
	require.False(t, op.PatternIR.Kind != "token" || op.PatternIR.Text != "interface{}",
		"%+v", op.PatternIR)

}

func TestResolvePatternIRPrefersSexp(t *testing.T) {
	op := Op{
		Mode:        "grep",
		PatternIR:   Node{Kind: "lit", Text: "old"},
		PatternSexp: json.RawMessage(`["token","interface{}"]`),
	}
	err := op.ResolvePatternIR()
	require.NoError(t, err)
	require.Equal(t, "interface{}", op.PatternIR.Text,
		"sexpr should win: %+v", op.PatternIR)

}

func TestCorePat_RejectsPatternIROnly(t *testing.T) {
	op := Op{Mode: "grep", PatternIR: Node{Kind: "token", Text: "x"}}
	_, err := op.CorePat()
	require.Error(t, err,
		"want error for pattern_ir-only op")

}
