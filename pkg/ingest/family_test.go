package ingest_test

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"
)

// Root tests use synthetic families only — no production surface names.

func TestRegisterFamily(t *testing.T) {
	const famID = "testfam_registry"
	const lang = "testlang_registry"

	f := ingest.RegisterFamily(famID, ingest.FamilySpec{})
	require.Equal(t, famID, f.ID())
	gotFam, ok := ingest.FamilyByID(famID)
	require.True(t, ok, "FamilyByID")
	require.Equal(t, famID, gotFam.ID())
	require.True(t, ingest.IsKnownFamily(famID),
		"IsKnownFamily")

	claims := []project.FamilyClaim{{Lang: lang, Family: famID}}
	got := project.FamilyIDForLanguage(claims, lang)
	require.Equal(t, famID, got)

	langs := project.LanguagesInFamily(claims, famID)
	require.Equal(t, []string{lang}, langs)
	require.True(t, project.LanguageInFamily(claims, lang, famID),
		"LanguageInFamily")

	fams := ingest.Families()
	foundFam := false
	for _, id := range fams {
		if id == famID {
			foundFam = true
			break
		}
	}
	require.True(t, foundFam,
		"Families() missing %q: %v", famID, fams)

}

func TestLanguageInFamilyConflictClaims(t *testing.T) {
	const famA = "testfam_conflict_a"
	const famB = "testfam_conflict_b"
	const lang = "testlang_conflict"
	_ = ingest.RegisterFamily(famA, ingest.FamilySpec{})
	_ = ingest.RegisterFamily(famB, ingest.FamilySpec{})
	claims := []project.FamilyClaim{{Lang: lang, Family: famA}}
	require.False(t, project.LanguageInFamily(claims, lang, famB),
		"lang claimed by A must not be in B")
	require.True(t, project.LanguageInFamily(claims, lang, famA),
		"lang claimed by A")

}
