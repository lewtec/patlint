package java_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/java"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestClassifyLeaf_Java(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"class":                   ingest.HLKeyword,
		"public":                  ingest.HLKeyword,
		"return":                  ingest.HLKeyword,
		"identifier":              ingest.HLIdent,
		"type_identifier":         ingest.HLType,
		"string_literal":          ingest.HLString,
		"text_block":              ingest.HLString,
		"decimal_integer_literal": ingest.HLNumber,
		"true":                    ingest.HLConst,
		"null":                    ingest.HLConst,
		"->":                      ingest.HLOp,
		"::":                      ingest.HLOp,
	}
	for in, want := range cases {
		got := ingest.ClassifyLeafLanguage(vm, "java", in)
		if got != want {
			t.Errorf("ClassifyLeaf(%q)=%q want %q", in, got, want)
		}
	}
	if got := ingest.ClassifyLeafLanguage(vm, "java", "("); got != ingest.HLPunct {
		t.Fatalf("punct: %q", got)
	}
}
