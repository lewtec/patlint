package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestJSNamedImportExtract(t *testing.T) {
	src := []byte("import { helper as h } from \"./helper.js\";\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(src, "main.js")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "main.js")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, im := range fe.Imports {
		t.Logf("local=%q src=%q member=%q alias=%v tgt=%d-%d", im.LocalName, im.SourcePath, im.MemberName, im.HasAliasBinding, im.TargetStartByte, im.TargetEndByte)
		if im.LocalName == "h" && im.MemberName == "helper" && im.SourcePath == "./helper.js" {
			found = true
			if !im.HasAliasBinding || im.TargetEndByte == 0 {
				t.Fatalf("want alias binding with target span: %+v", im)
			}
		}
	}
	if !found {
		t.Fatalf("missing named import, imports=%d", len(fe.Imports))
	}
}
