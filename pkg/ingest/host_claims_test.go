package ingest_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	_ "github.com/lewtec/patlint/pkg/ingest/c"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
)

func TestSameFamilyCAndCpp(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	if !project.SameFamily(vm.Families(), "c", "cpp") {
		t.Fatal("c and cpp must share a family")
	}
}
