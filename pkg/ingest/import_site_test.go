package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
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
	if len(file.sites) != 1 {
		t.Fatalf("sites=%d", len(file.sites))
	}
	site := file.sites[0]
	if site.specifier != "pkg" || !site.keepsOther("helper") || site.aliasOf("helper") != "h" {
		t.Fatalf("site spec=%q members=%v", site.specifier, site.members)
	}
	e, ok := site.dropMember(src, "helper", "from .b import helper as h\n")
	if !ok {
		t.Fatal("drop")
	}
	if e.NewText != "from pkg import stay\nfrom .b import helper as h\n" {
		t.Fatalf("drop text %q", e.NewText)
	}
	e, ok = site.rewriteMember(src, "helper", "helper_fuzz")
	if !ok || e.NewText != "helper_fuzz" {
		t.Fatalf("rewrite ok=%v text %q", ok, e.NewText)
	}
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
	if len(file.sites) != 1 || len(file.sites[0].members) != 2 {
		t.Fatalf("sites=%d members=%v", len(file.sites), file.sites)
	}
	if !file.sites[0].keepsOther("helper") {
		t.Fatalf("members=%v", file.sites[0].members)
	}
}

func TestImportSiteBareImportIsNotFrom(t *testing.T) {
	src := []byte("import os\n")
	fe := &project.FileExtract{
		Path:    "a.py",
		Imports: []project.ImportDef{{SourcePath: "os", LocalName: "os", StartByte: 0, EndByte: 8}},
	}
	file := loadImportFile(src, fe)
	if len(file.sites) != 1 || file.sites[0].from || len(file.bindings()) != 0 {
		t.Fatalf("bare import treated as from: %+v bindings=%v", file.sites, file.bindings())
	}
}
