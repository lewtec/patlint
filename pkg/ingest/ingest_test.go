package ingest_test

import (
	"errors"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/script"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestProjectResult_Fixtures(t *testing.T) {
	cases, err := script.LoadCases(t.Context(), lewpath.New("..", "..", "testdata", "ingest").String())
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no testdata/ingest cases")
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

func TestExtractFile_NilPolicy(t *testing.T) {
	fe, err := ingest.ExtractFile(t.Context(), nil, project.NewSession(".").WithEngine(ccgo.Engine{}), "go", nil, nil, "x.go")
	if !errors.Is(err, ingest.ErrNilPolicy) || fe != nil {
		t.Fatalf("fe=%v err=%v", fe, err)
	}
}
