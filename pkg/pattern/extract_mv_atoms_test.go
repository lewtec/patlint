package pattern_test

import (
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/script"
)

func TestExtract_Fixtures(t *testing.T) {
	cases, err := script.LoadCases(t.Context(), lewpath.New("..", "..", "testdata", "extract").String())
	require.NoError(t, err)
	require.NotEmpty(t, cases,
		"no testdata/extract cases")

	for _, c := range cases {
		t.Run(c.Rel(lewpath.New("..", "..").String()), func(t *testing.T) {
			res, err := script.RunCase(t.Context(), c, script.RunCaseOptions{})
			require.NoError(t, err)

			for _, f := range res.Failures {
				t.Error(f)
			}
		})
	}
}
