package ingest_test

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
)

// Root tests use synthetic families only — no production surface names.

func TestRegisterFamily(t *testing.T) {
	const famID = "testfam_registry"
	const lang = "testlang_registry"

	f := ingest.RegisterFamily(famID, ingest.FamilySpec{})
	if f.ID() != famID {
		t.Fatalf("ID=%q", f.ID())
	}
	if got, ok := ingest.FamilyByID(famID); !ok || got.ID() != famID {
		t.Fatal("FamilyByID")
	}
	if !ingest.IsKnownFamily(famID) {
		t.Fatal("IsKnownFamily")
	}
	claims := []project.FamilyClaim{{Lang: lang, Family: famID}}
	if got := project.FamilyIDForLanguage(claims, lang); got != famID {
		t.Fatalf("FamilyIDForLanguage=%q", got)
	}
	langs := project.LanguagesInFamily(claims, famID)
	if len(langs) != 1 || langs[0] != lang {
		t.Fatalf("LanguagesInFamily=%v", langs)
	}
	if !project.LanguageInFamily(claims, lang, famID) {
		t.Fatal("LanguageInFamily")
	}
	fams := ingest.Families()
	foundFam := false
	for _, id := range fams {
		if id == famID {
			foundFam = true
			break
		}
	}
	if !foundFam {
		t.Fatalf("Families() missing %q: %v", famID, fams)
	}
}

func TestLanguageInFamilyConflictClaims(t *testing.T) {
	const famA = "testfam_conflict_a"
	const famB = "testfam_conflict_b"
	const lang = "testlang_conflict"
	_ = ingest.RegisterFamily(famA, ingest.FamilySpec{})
	_ = ingest.RegisterFamily(famB, ingest.FamilySpec{})
	claims := []project.FamilyClaim{{Lang: lang, Family: famA}}
	if project.LanguageInFamily(claims, lang, famB) {
		t.Fatal("lang claimed by A must not be in B")
	}
	if !project.LanguageInFamily(claims, lang, famA) {
		t.Fatal("lang claimed by A")
	}
}
