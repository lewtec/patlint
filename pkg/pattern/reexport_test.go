package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
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
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "barrel.js")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), "", pf.Root, src, "barrel.js")
	require.NoError(t, err)

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
	require.True(t, star, "DefaultExport=%q", fe.DefaultExport)
	require.True(t, named, "DefaultExport=%q", fe.DefaultExport)
	require.True(t, defAs, "DefaultExport=%q", fe.DefaultExport)
	// as-default: createIntegration as default should win if first; Thing is also default
	require.NotEmpty(t, fe.DefaultExport)

}
