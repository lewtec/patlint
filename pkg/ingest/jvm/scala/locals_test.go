package scala_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/scala"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestScalaLocalBindings(t *testing.T) {
	src := []byte(`object Main {
  def f(a: Int): Int = {
    val b = a + 1
    b
  }
}
`)
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, src, "Main.scala", "scala")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	idx := ingest.LocalBindingsForLanguage(t.Context(), vm, project.NewSession(".").WithEngine(ccgo.Engine{}), pf.Root, src, "scala", "Main.scala")
	if idx == nil {
		t.Fatal("nil binding index")
	}
}
