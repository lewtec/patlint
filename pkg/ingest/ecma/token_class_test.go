package ecma_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/ecma/js"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestClassifyLeaf_ECMA(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"function":        ingest.HLKeyword,
		"const":           ingest.HLKeyword,
		"identifier":      ingest.HLIdent,
		"string":          ingest.HLString,
		"template_string": ingest.HLString,
		"number":          ingest.HLNumber,
		"type_identifier": ingest.HLType,
		"true":            ingest.HLConst,
		"undefined":       ingest.HLConst,
		"=>":              ingest.HLOp,
	}
	for in, want := range cases {
		got := ingest.ClassifyLeafLanguage(vm, "javascript", in)
		if got != want {
			t.Errorf("ClassifyLeaf(%q)=%q want %q", in, got, want)
		}
	}
	if got := ingest.ClassifyLeafLanguage(vm, "javascript", ""); got != "" {
		t.Fatalf("empty=%q", got)
	}
}

func TestClassifyLeaf_ViaDrivers(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "javascript", "function"); got != ingest.HLKeyword {
		t.Fatalf("js function: %q", got)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "typescript", "type_identifier"); got != ingest.HLType {
		t.Fatalf("ts type: %q", got)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "svelte", "tag_name"); got != ingest.HLType {
		t.Fatalf("svelte tag: %q", got)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "vue", "tag_name"); got != ingest.HLType {
		t.Fatalf("vue tag: %q", got)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "astro", "tag_name"); got != ingest.HLType {
		t.Fatalf("astro tag: %q", got)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "javascript", "("); got != ingest.HLPunct {
		t.Fatalf("js punct: %q", got)
	}
}
