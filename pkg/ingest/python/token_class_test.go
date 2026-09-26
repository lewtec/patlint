package python_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/python"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestClassifyLeaf_Python(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"def":        ingest.HLKeyword,
		"class":      ingest.HLKeyword,
		"import":     ingest.HLKeyword,
		"identifier": ingest.HLIdent,
		"string":     ingest.HLString,
		"integer":    ingest.HLNumber,
		"True":       ingest.HLConst,
		"None":       ingest.HLConst,
		":=":         ingest.HLOp,
	}
	for in, want := range cases {
		got := ingest.ClassifyLeafLanguage(vm, "python", in)
		if got != want {
			t.Errorf("ClassifyLeaf(%q)=%q want %q", in, got, want)
		}
	}
	if got := ingest.ClassifyLeafLanguage(vm, "python", "("); got != ingest.HLPunct {
		t.Fatalf("punct: %q", got)
	}
}
