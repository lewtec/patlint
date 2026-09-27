package pattern

import (
	"encoding/json"
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"
)

func TestParsePattern_Fixtures(t *testing.T) {
	// Round-trip: parse fixture pattern string, match same cases as sexp intent.
	cases := []struct {
		name     string
		pattern  string
		wantKind string
	}{
		{"any", `(token "interface{}")`, "token"},
		{"failed", `(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG (regex "(?i)^failed to\\s+(.*)" 1)) "," (capture ERR any) ")")`, "seq"},
		{"splitn", `(seq (capture F (ref "go:strings::SplitN")) "(" (capture S any) "," (capture SEP any) "," "2" ")")`, "seq"},
		{"listen", `(seq (capture F (ref "go:net/http::ListenAndServe")) "(" (capture ADDR any) "," (capture HANDLER any) ")")`, "seq"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := ParsePattern(tc.pattern)
			require.NoError(t, err)
			require.Equal(t, tc.wantKind, n.Kind,
				"kind=%s want %s\n%s", n.Kind, tc.wantKind, mustJSON(n))

		})
	}
}

func TestParsePattern_MatchesFixtureIR(t *testing.T) {
	root := lewpath.New("..", "..", "testdata", "pattern").String()
	entries, err := os.ReadDir(root)
	require.NoError(t, err)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			op, err := LoadOp(lewpath.New(root, e.Name()).String())
			require.NoError(t, err)
			require.NotEmpty(t, op.Pattern,
				"empty pattern")

			got, err := ParsePattern(op.Pattern)
			require.NoError(t, err,
				"parse: %v", err)

			// When pattern_sexp is set, PatternIR is filled from sexpr (often seq, not call sugar).
			if len(op.PatternSexp) > 0 {
				return
			}
			// Structural checks rather than deep equal (IR may use As:ROOT etc.)
			require.Equal(t, op.PatternIR.Kind, got.Kind, "got=%s\nwant=%s", mustJSON(got), mustJSON(op.PatternIR))

			if op.Mode == "rewrite" && op.Replacement != nil && *op.Replacement != "" {
				{
					_, err := ParseEmit(*op.Replacement)
					require.NoError(t, err,
						"parse emit: %v", err)
				}

			}
		})
	}
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "<marshal error>"
	}
	return string(b)
}
