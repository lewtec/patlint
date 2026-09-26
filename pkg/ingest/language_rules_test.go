package ingest_test

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestLanguageRulesFromPrelude(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	goR := vm.RulesForLanguage("go")
	if !goR.DirectoryModule || !goR.PackageScopedBareNames || !goR.NestedTypeMembers {
		t.Fatalf("go=%#v", goR)
	}
	if got := ingestutil.EmitSeq(vm.PackageLineInfo("go").Tokens, map[string]string{"pkg": "main"}); got != "package main" {
		t.Fatalf("go package seq=%q", got)
	}
	if got := strings.Join(vm.Layout("go"), " "); got != "package import body" {
		t.Fatalf("go layout=%q", got)
	}
	if got := strings.Join(vm.Layout("python"), " "); got != "import body" {
		t.Fatalf("python layout=%q", got)
	}
	if got := strings.Join(vm.Layout("java"), " "); got != "package import body" {
		t.Fatalf("java layout=%q", got)
	}
	if got := vm.Layout("nix"); len(got) != 0 {
		t.Fatalf("nix layout=%q", got)
	}
	if got := strings.Join(vm.Layout("c"), " "); got != "package import body" {
		t.Fatalf("c layout=%q", got)
	}
	java := vm.RulesForLanguage("java")
	if !java.DirectoryModule || !java.EmptyPackageDirScoped {
		t.Fatalf("java=%#v", java)
	}
	c := vm.RulesForLanguage("c")
	if !c.IncludeFileExportsBare || !c.NestedTypeMembers {
		t.Fatalf("c=%#v", c)
	}
	js := vm.RulesForLanguage("javascript")
	if js.DirectoryModule {
		t.Fatal("ecma directory-module")
	}
	if !js.DirectoryManifest {
		t.Fatal("ecma directory-manifest")
	}
	if !js.DestExport {
		t.Fatal("ecma dest-export")
	}
	py := vm.RulesForLanguage("python")
	if !py.RejectDunderRename {
		t.Fatal("python reject-dunder-rename")
	}
}
