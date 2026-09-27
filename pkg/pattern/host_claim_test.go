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

func TestHostOnlyPackClaimsPath(t *testing.T) {
	src := `(under (path "**/*.rft") (as-language "commonlisp"))`
	prog, err := pattern.LoadExtractPack("language_rft.rft", src)
	require.NoError(t, err)
	require.Equal(t, "commonlisp", prog.Language)
	require.NotEmpty(t, prog.Actions, "host-only pack must emit a path claim action")

	found := false
	for _, a := range prog.Actions {
		if a.HostLang == "commonlisp" && a.Matcher == nil {
			found = true
			break
		}
	}
	require.True(t, found, "want Matcher-nil host claim, got %+v", prog.Actions)

}

func TestProductClaimsRftAndLoadsCommonPaint(t *testing.T) {
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	p := vm.Packs().Program()
	require.NotNil(t, p)

	lang, ok := vm.HostLanguage("demo.rft")
	require.True(t, ok)
	require.Equal(t, "commonlisp", lang)

	var paint int
	for _, a := range p.Actions {
		if a.Kind == pattern.ExtractPaint {
			paint++
		}
	}
	require.NotZero(t, paint, "program has no paint actions (language_common/highlight not loaded?)")

	src := []byte("(under (path \"**/*.go\") (as-language \"go\"))\n")
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	cells, gotLang, err := w.BuildTape(t.Context(), src, "demo.rft")
	require.NoError(t, err)
	require.Equal(t, "commonlisp", gotLang)
	require.NotEmpty(t, cells)

}
