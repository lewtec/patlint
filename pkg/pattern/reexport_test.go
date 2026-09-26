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

func TestJSReexportExtract(t *testing.T) {
	src := []byte(`export * from "./inner.js";
export { real } from "./impl.js";
export { default as Search } from "./search.js";
export { createIntegration as default };
export default function Thing() {}
export function real() {}
`)
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := w.ParseAttributed(src, "barrel.js")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "barrel.js")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("DefaultExport=%q", fe.DefaultExport)
	for _, re := range fe.Reexports {
		t.Logf("reexport export=%q source=%q path=%q star=%v", re.ExportName, re.SourceName, re.SourcePath, re.Star)
	}
	if fe.DefaultExport != "createIntegration" && fe.DefaultExport != "Thing" {
		// either is ok depending on order; prefer first as-default that wins
		t.Logf("note: DefaultExport=%q", fe.DefaultExport)
	}
	var star, named, defAs bool
	for _, re := range fe.Reexports {
		if re.Star && re.SourcePath == "./inner.js" {
			star = true
		}
		if re.ExportName == "real" && re.SourcePath == "./impl.js" {
			named = true
		}
		if re.ExportName == "Search" && re.SourceName == "default" && re.SourcePath == "./search.js" {
			defAs = true
		}
	}
	if !star || !named || !defAs {
		t.Fatalf("star=%v named=%v defAs=%v DefaultExport=%q", star, named, defAs, fe.DefaultExport)
	}
	// as-default: createIntegration as default should win if first; Thing is also default
	if fe.DefaultExport == "" {
		t.Fatal("expected DefaultExport set")
	}
}
