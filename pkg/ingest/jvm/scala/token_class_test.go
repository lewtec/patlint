package scala_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/scala"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestScalaTokenClass(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "scala", "identifier"); got != ingest.HLIdent {
		t.Fatalf("identifier=%q", got)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "scala", "class"); got != ingest.HLKeyword {
		t.Fatalf("class=%q", got)
	}
	if got := ingest.ClassifyLeafLanguage(vm, "scala", "string"); got != ingest.HLString {
		t.Fatalf("string=%q", got)
	}
}
