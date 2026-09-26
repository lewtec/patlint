package pattern

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func parsePat(t *testing.T, s string) Pat {
	t.Helper()
	core, err := ParseToPat(s)
	require.NoError(t, err)

	return core
}

func TestAnalyzeStartPreds_lit(t *testing.T) {
	n, err := compilePat(parsePat(t, `(token "interface{}")`))
	require.NoError(t, err)
	require.Len(t, n.startPreds, 1)
	require.Contains(t, []predKind{predToken, predLit}, n.startPreds[0].kind)
	require.Equal(t, "interface{}", n.startPreds[0].text)

}

func TestAnalyzeStartPreds_anyDisables(t *testing.T) {
	// Rest / any first atom must not filter (would miss valid starts).
	n, err := compilePat(parsePat(t, `(seq (* any) (token "interface{}"))`))
	require.NoError(t, err)
	require.Nil(t, n.startPreds, "want no filter for leading rest")

}

func TestAnalyzeStartPreds_discardedBlank(t *testing.T) {
	op, err := LoadOp("../../testdata/pattern/go_discarded_error_blank")
	require.NoError(t, err)

	core, err := op.CorePat()
	require.NoError(t, err)

	n, err := compilePat(core)
	require.NoError(t, err)
	require.NotEmpty(t, n.startPreds, "want start filter on discarded-error pattern")

	// First atom is (capture blank (regex "^_$")) → regex
	var kinds []predKind
	for _, p := range n.startPreds {
		kinds = append(kinds, p.kind)
	}
	require.Contains(t, kinds, predRegex)

}
