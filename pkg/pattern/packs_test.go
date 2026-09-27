package pattern_test

import (
	"testing"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

	lang, ok, err := p.HostLanguage("app.js")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "javascript", lang)
	require.Equal(t, "ecma", p.FamilyForLanguage("javascript"))
	require.False(t, p.RulesForLanguage("javascript").DirectoryModule,
		"ecma directory-module")

	goPack, err := pattern.LoadFiles([]pattern.PackFile{{
		Name: "go.rft",
		Src: `
(under (path "**/*.go")
  (as-language "go")
  (as-family "go" directory-module package-scoped-bare-names nested-type-members))
`,
	}})
	require.NoError(t, err)
	require.Equal(t, "go", goPack.FamilyForLanguage("go"))

	got := goPack.RulesForLanguage("go")
	require.True(t, got.DirectoryModule, "go rules=%#v", got)
	require.True(t, got.NestedTypeMembers, "go rules=%#v", got)

}
