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

func TestPythonFromImportExtract(t *testing.T) {
	cases := []struct {
		src        string
		wantLocal  string
		wantPath   string
		wantMember string
	}{
		{"from .localmod import helper\n", "helper", ".localmod", "helper"},
		{"from pkg.sub import helper\n", "helper", "pkg.sub", "helper"},
		{"from pkg.sub import helper as h\n", "h", "pkg.sub", "helper"},
		{"from pkg import stay, helper\n", "stay", "pkg", "stay"},
		{"import os\n", "os", "os", ""},
	}
	for _, tc := range cases {
		vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
		require.NoError(t, err)

		w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
		require.NoError(t, err)

		pf, lang, err := w.ParseAttributed(t.Context(), []byte(tc.src), "pkg/app.py")
		require.NoError(t, err,
			"%q: %v", tc.src, err)
		require.Equal(t, "python", lang,
			"lang=%q", lang)

		fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, []byte(tc.src), "pkg/app.py")
		pf.Close()
		require.NoError(t, err)
		require.NotEmpty(t, fe.Imports,
			"%q: no imports", tc.src)

		im := fe.Imports[0]
		if im.LocalName != tc.wantLocal || im.SourcePath != tc.wantPath || im.MemberName != tc.wantMember {
			t.Errorf("%q: local=%q src=%q member=%q want local=%q src=%q member=%q",
				tc.src, im.LocalName, im.SourcePath, im.MemberName, tc.wantLocal, tc.wantPath, tc.wantMember)
		}
	}
}

func TestPythonFromImportFansOutNames(t *testing.T) {
	src := []byte("from pkg import stay, helper as h\n")
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	pf, _, err := w.ParseAttributed(t.Context(), src, "pkg/app.py")
	require.NoError(t, err)

	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, src, "pkg/app.py")
	pf.Close()
	require.NoError(t, err)

	got := map[string]string{}
	for _, im := range fe.Imports {
		if im.MemberName != "" {
			got[im.MemberName] = im.LocalName
		}
	}
	require.False(t, got["stay"] != "stay" || got["helper"] != "h",
		"members=%v", got)

}
