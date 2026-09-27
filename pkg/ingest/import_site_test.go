package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"
)

func TestImportSiteDropAndRewrite(t *testing.T) {
	src := []byte("from pkg import stay, helper as h\n")
	fe := &project.FileExtract{
		Path: "a.py",
		Imports: []project.ImportDef{
			{SourcePath: "pkg", MemberName: "stay", StartByte: 16, EndByte: 20},
			{SourcePath: "pkg", MemberName: "helper", LocalName: "h", HasAliasBinding: true, StartByte: 22, EndByte: 33},
		},
	}
	file := loadImportFile(src, fe)
	require.Len(t, file.sites, 1)

	site := file.sites[0]
	require.Equal(t, "pkg", site.specifier)
	require.True(t, site.keepsOther("helper"), "members=%v", site.members)
	require.Equal(t, "h", site.aliasOf("helper"))

	e, ok := site.dropMember(src, "helper", "from .b import helper as h\n")
	require.True(t, ok, "drop")
	require.Equal(t, "from pkg import stay\nfrom .b import helper as h\n", e.NewText)

	e, ok = site.rewriteMember(src, "helper", "helper_fuzz")
	require.True(t, ok)
	require.Equal(t, "helper_fuzz", e.NewText)

}

func TestImportSitePrefersExtractMembers(t *testing.T) {
	src := []byte("from pkg import stay, helper\n")
	fe := &project.FileExtract{
		Path: "a.py",
		Imports: []project.ImportDef{
			{SourcePath: "pkg", MemberName: "stay", StartByte: 16, EndByte: 20},
			{SourcePath: "pkg", MemberName: "helper", StartByte: 22, EndByte: 28},
		},
	}
	file := loadImportFile(src, fe)
	require.Len(t, file.sites, 1)
	require.Len(t, file.sites[0].members, 2)
	require.True(t, file.sites[0].keepsOther("helper"))

}

func TestImportSiteBareImportIsNotFrom(t *testing.T) {
	src := []byte("import os\n")
	fe := &project.FileExtract{
		Path:    "a.py",
		Imports: []project.ImportDef{{SourcePath: "os", LocalName: "os", StartByte: 0, EndByte: 8}},
	}
	file := loadImportFile(src, fe)
	require.Len(t, file.sites, 1)
	require.False(t, file.sites[0].from, "bare import treated as from: %+v", file.sites)
	require.Empty(t, file.bindings())

}
