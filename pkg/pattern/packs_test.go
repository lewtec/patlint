package pattern_test

import (
	"testing"

	_ "github.com/lewtec/patlint/pkg/ingest/ecma"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestLoadFilesHostAndFamily(t *testing.T) {
	p, err := pattern.LoadFiles([]pattern.PackFile{{
		Name: "t.rft",
		Src: `
(under (path "**/*.js")
  (as-language "javascript")
  (as-family "ecma"))
`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	lang, ok, err := p.HostLanguage("app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || lang != "javascript" {
		t.Fatalf("host=%q ok=%v", lang, ok)
	}
	if got := p.FamilyForLanguage("javascript"); got != "ecma" {
		t.Fatalf("family=%q", got)
	}
	if p.RulesForLanguage("javascript").DirectoryModule {
		t.Fatal("ecma directory-module")
	}
	goPack, err := pattern.LoadFiles([]pattern.PackFile{{
		Name: "go.rft",
		Src: `
(under (path "**/*.go")
  (as-language "go")
  (as-family "go" directory-module package-scoped-bare-names nested-type-members))
`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := goPack.FamilyForLanguage("go"); got != "go" {
		t.Fatalf("go family=%q", got)
	}
	got := goPack.RulesForLanguage("go")
	if !got.DirectoryModule || !got.NestedTypeMembers {
		t.Fatalf("go rules=%#v", got)
	}
}
