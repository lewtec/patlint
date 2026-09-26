package ecma_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest/ecma"
	_ "github.com/lewtec/patlint/pkg/ingest/ecma/js"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestECMASurfacesJoinFamily(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	wantLangs := []string{"javascript", "typescript", "tsx", "svelte", "vue", "astro"}
	for _, lang := range wantLangs {
		if got := vm.FamilyForLanguage(lang); got != ecma.FamilyID {
			t.Fatalf("%s family=%q", lang, got)
		}
	}
	if vm.FamilyForLanguage("javascript") != vm.FamilyForLanguage("typescript") || vm.FamilyForLanguage("typescript") != vm.FamilyForLanguage("svelte") {
		t.Fatal("SameFamily")
	}
	langs := vm.LanguagesInFamily(ecma.FamilyID)
	have := map[string]bool{}
	for _, l := range langs {
		have[l] = true
	}
	for _, lang := range wantLangs {
		if !have[lang] {
			t.Fatalf("langs=%v missing %s", langs, lang)
		}
	}
	for _, tc := range []struct{ file, want string }{
		{"App.svelte", "svelte"},
		{"App.vue", "vue"},
		{"Page.astro", "astro"},
		{"main.ts", "typescript"},
		{"main.tsx", "tsx"},
		{"main.js", "javascript"},
	} {
		lang, ok := vm.HostLanguage(tc.file)
		if !ok || lang != tc.want {
			t.Fatalf("%s: lang=%q ok=%v want %q", tc.file, lang, ok, tc.want)
		}
		if vm.FamilyForLanguage(lang) != ecma.FamilyID {
			t.Fatalf("%s family=%q", tc.file, vm.FamilyForLanguage(lang))
		}
	}
}
