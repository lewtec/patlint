package ingest_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

type docFixtureCase struct {
	Name              string `json:"name"`
	Fixture           string `json:"fixture"`
	Reference         string `json:"reference"`
	ExpectError       bool   `json:"expect_error"`
	ExpectName        string `json:"expect_name"`
	ExpectDocContains string `json:"expect_doc_contains"`
}

type textAssertion struct {
	File string `json:"file"`
	Text string `json:"text"`
}

type renameFixtureCase struct {
	Name                   string          `json:"name"`
	Fixture                string          `json:"fixture"`
	Source                 string          `json:"source"`
	Destination            string          `json:"destination"`
	ExpectError            bool            `json:"expect_error"`
	ExpectEditCountAtLeast int             `json:"expect_edit_count_at_least"`
	ApplyEdits             bool            `json:"apply_edits"`
	Contains               []textAssertion `json:"contains"`
	NotContains            []textAssertion `json:"not_contains"`
}

func TestDocFor_ReferenceFixtures(t *testing.T) {
	fixtureRoot := lewpath.New("..", "..", "testdata", "reference").String()
	input, err := os.ReadFile(lewpath.New(fixtureRoot, "doc_cases.json").String())
	if err != nil {
		t.Fatalf("reading doc_cases.json: %v", err)
	}

	var cases []docFixtureCase
	if err := json.Unmarshal(input, &cases); err != nil {
		t.Fatalf("parsing doc_cases.json: %v", err)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			dir := lewpath.New(fixtureRoot, tc.Fixture).String()
			vm, err := pattern.New(prelude.FS)
			if err != nil {
				t.Fatal(err)
			}
			w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := w.Doc(t.Context(), dir, tc.Reference)

			if tc.ExpectError {
				if err == nil {
					t.Fatalf("expected error, got doc %+v", doc)
				}
				return
			}

			if err != nil {
				t.Fatalf("doc lookup failed: %v", err)
			}
			if tc.ExpectName != "" && doc.Name != tc.ExpectName {
				t.Fatalf("unexpected name: got %q want %q", doc.Name, tc.ExpectName)
			}
			if tc.ExpectDocContains != "" && !strings.Contains(doc.DocString, tc.ExpectDocContains) {
				t.Fatalf("expected docstring to contain %q, got %q", tc.ExpectDocContains, doc.DocString)
			}
		})
	}
}

func TestRename_ReferenceFixtures(t *testing.T) {
	fixtureRoot := lewpath.New("..", "..", "testdata", "reference").String()
	input, err := os.ReadFile(lewpath.New(fixtureRoot, "rename_cases.json").String())
	if err != nil {
		t.Fatalf("reading rename_cases.json: %v", err)
	}

	var cases []renameFixtureCase
	if err := json.Unmarshal(input, &cases); err != nil {
		t.Fatalf("parsing rename_cases.json: %v", err)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			srcDir := lewpath.New(fixtureRoot, tc.Fixture).String()
			tmpDir := t.TempDir()
			copyDir(t, srcDir, tmpDir)

			vm, err := pattern.New(prelude.FS)
			if err != nil {
				t.Fatal(err)
			}
			w, err := walker.NewWalker(project.NewSession(tmpDir).WithEngine(ccgo.Engine{}), vm)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := w.Rename(t.Context(), tmpDir, tc.Source, tc.Destination)
			if tc.ExpectError {
				if err == nil {
					t.Fatalf("expected error, got %d edits", len(plan.Edits))
				}
				return
			}
			if err != nil {
				t.Fatalf("rename failed: %v", err)
			}
			if tc.ExpectEditCountAtLeast > 0 && len(plan.Edits) < tc.ExpectEditCountAtLeast {
				t.Fatalf("expected at least %d edits, got %d", tc.ExpectEditCountAtLeast, len(plan.Edits))
			}

			if !tc.ApplyEdits {
				return
			}
			if err := ingest.ApplyPlan(t.Context(), tmpDir, plan); err != nil {
				t.Fatalf("apply edits failed: %v", err)
			}

			for _, check := range tc.Contains {
				content, err := os.ReadFile(lewpath.New(tmpDir, check.File).String())
				if err != nil {
					t.Fatalf("reading %s: %v", check.File, err)
				}
				if !strings.Contains(string(content), check.Text) {
					t.Fatalf("expected %s to contain %q, got:\n%s", check.File, check.Text, content)
				}
			}

			for _, check := range tc.NotContains {
				content, err := os.ReadFile(lewpath.New(tmpDir, check.File).String())
				if err != nil {
					t.Fatalf("reading %s: %v", check.File, err)
				}
				if strings.Contains(string(content), check.Text) {
					t.Fatalf("expected %s to not contain %q, got:\n%s", check.File, check.Text, content)
				}
			}
		})
	}
}
