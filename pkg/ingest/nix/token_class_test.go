package nix_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/nix"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestClassifyLeaf_Nix(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"let":        ingest.HLKeyword,
		"in":         ingest.HLKeyword,
		"inherit":    ingest.HLKeyword,
		"identifier": ingest.HLIdent,
		"string":     ingest.HLString,
		"path":       ingest.HLString,
		"integer":    ingest.HLNumber,
		"true":       ingest.HLConst,
		"//":         ingest.HLOp,
	}
	for in, want := range cases {
		got := ingest.ClassifyLeafLanguage(vm, "nix", in)
		if got != want {
			t.Errorf("ClassifyLeaf(%q)=%q want %q", in, got, want)
		}
	}
}
