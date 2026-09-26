package kotlin_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/kotlin"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestKotlinTokenClass(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "kotlin", "simple_identifier"); got != ingest.HLIdent {
		t.Fatalf("simple_identifier=%q", got)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "kotlin", "fun"); got != ingest.HLKeyword {
		t.Fatalf("fun=%q", got)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "kotlin", "string_literal"); got != ingest.HLString {
		t.Fatalf("string_literal=%q", got)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "kotlin", "type_identifier"); got != ingest.HLType {
		t.Fatalf("type_identifier=%q", got)
	}
}
