package pattern_test

import (
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/script"
)

func TestExtract_Fixtures(t *testing.T) {
	cases, err := script.LoadCases(t.Context(), lewpath.New("..", "..", "testdata", "extract").String())
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no testdata/extract cases")
	}
	for _, c := range cases {
		t.Run(c.Rel(lewpath.New("..", "..").String()), func(t *testing.T) {
			res, err := script.RunCase(t.Context(), c, script.RunCaseOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range res.Failures {
				t.Error(f)
			}
		})
	}
}
