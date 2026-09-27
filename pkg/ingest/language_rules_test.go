package ingest_test

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/stretchr/testify/require"
)

func TestLanguageRulesFromPrelude(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	goR := vm.RulesForLanguage("go")
	require.False(t, !goR.DirectoryModule || !goR.PackageScopedBareNames || !goR.NestedTypeMembers,
		"go=%#v", goR)
	{

		got := ingestutil.EmitSeq(vm.PackageLineInfo("go").Tokens, map[string]string{"pkg": "main"})
		require.Equal(t, "package main", got,
			"go package seq=%q", got)
	}
	{

		got := strings.Join(vm.Layout("go"), " ")
		require.Equal(t, "package import body", got,
			"go layout=%q", got)
	}
	{

		got := strings.Join(vm.Layout("python"), " ")
		require.Equal(t, "import body", got,
			"python layout=%q", got)
	}
	{

		got := strings.Join(vm.Layout("java"), " ")
		require.Equal(t, "package import body", got,
			"java layout=%q", got)
	}
	{

		got := vm.Layout("nix")
		require.Empty(t, got,
			"nix layout=%q", got)
	}

	got := strings.Join(vm.Layout("c"), " ")
	require.Equal(t, "package import body", got,
		"c layout=%q", got)

	java := vm.RulesForLanguage("java")
	require.False(t, !java.DirectoryModule || !java.EmptyPackageDirScoped,
		"java=%#v", java)

	c := vm.RulesForLanguage("c")
	require.False(t, !c.IncludeFileExportsBare || !c.NestedTypeMembers,
		"c=%#v", c)

	js := vm.RulesForLanguage("javascript")
	require.False(t, js.DirectoryModule,
		"ecma directory-module")
	require.True(t, js.DirectoryManifest,
		"ecma directory-manifest")
	require.True(t, js.DestExport,
		"ecma dest-export")

	py := vm.RulesForLanguage("python")
	require.True(t, py.RejectDunderRename,
		"python reject-dunder-rename")

}
