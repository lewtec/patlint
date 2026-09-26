package jvm_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingest/jvm"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/java"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/kotlin"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/scala"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestJavaJoinsJVMFamily(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	if got := vm.FamilyForLanguage("java"); got != jvm.FamilyID {
		t.Fatalf("java family=%q want %q", got, jvm.FamilyID)
	}
	langs := vm.LanguagesInFamily(jvm.FamilyID)
	if len(langs) < 1 {
		t.Fatalf("langs=%v", langs)
	}
	have := map[string]bool{}
	for _, l := range langs {
		have[l] = true
	}
	for _, lang := range []string{"java", "scala", "kotlin"} {
		if !have[lang] {
			t.Fatalf("langs=%v missing %s", langs, lang)
		}
	}
	if !vm.LanguageInFamily("java", jvm.FamilyID) {
		t.Fatal("LanguageInFamily")
	}
	if !vm.LanguageInFamily("scala", jvm.FamilyID) {
		t.Fatal("scala should join jvm family")
	}
	if !vm.LanguageInFamily("kotlin", jvm.FamilyID) {
		t.Fatal("kotlin should join jvm family")
	}
	for _, path := range []string{"Foo.java", "Foo.scala", "Foo.kt", "build.kts"} {
		lang, ok := vm.HostLanguage(path)
		if !ok || vm.FamilyForLanguage(lang) != jvm.FamilyID {
			t.Fatalf("%s host=%q ok=%v family=%q", path, lang, ok, vm.FamilyForLanguage(lang))
		}
	}
	if _, ok := ingest.LatticeForFamily(jvm.FamilyID); !ok {
		t.Fatal("lattice")
	}
}
