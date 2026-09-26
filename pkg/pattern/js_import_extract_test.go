package pattern_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestJSNamedImportExtract(t *testing.T) {
	src := []byte("import { helper as h } from \"./helper.js\";\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "main.js")
	require.NoError(t, err)

	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "main.js")
	require.NoError(t, err)

	var found bool
	for _, im := range fe.Imports {
		t.Logf("local=%q src=%q member=%q alias=%v tgt=%d-%d", im.LocalName, im.SourcePath, im.MemberName, im.HasAliasBinding, im.TargetStartByte, im.TargetEndByte)
		if im.LocalName == "h" && im.MemberName == "helper" && im.SourcePath == "./helper.js" {
			found = true
			require.True(t, im.HasAliasBinding, "want alias binding: %+v", im)
			require.NotZero(t, im.TargetEndByte, "want target span: %+v", im)
		}
	}
	require.True(t, found, "missing named import, imports=%d", len(fe.Imports))
}
