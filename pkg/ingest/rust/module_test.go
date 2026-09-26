package rust_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	ingestrust "github.com/lewtec/patlint/pkg/ingest/rust"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestFamilyRegistered(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	if ingestrust.FamilyID != "rust" {
		t.Fatalf("FamilyID=%q", ingestrust.FamilyID)
	}
	if _, ok := ingest.LatticeForFamily("rust"); !ok {
		t.Fatal("rust lattice not registered")
	}
	if vm.FamilyForLanguage("rust") != "rust" {
		t.Fatalf("family for rust: %q", vm.FamilyForLanguage("rust"))
	}
	lang, ok := vm.HostLanguage("foo.rs")
	if !ok || lang != "rust" {
		t.Fatalf("LanguageForFile .rs: %q ok=%v", lang, ok)
	}
	n, ok := vm.ImportNeedFromRef("rust", "rust:std::io::Result")
	if !ok || n.ImportPath != "std::io::Result" {
		t.Fatalf("rust import-ref-name: %+v ok=%v", n, ok)
	}
}

func TestResolveImport_ModAndCrate(t *testing.T) {
	ctx := ingest.ImportResolveContext{
		RootDir:      ".",
		ImporterPath: "main.rs",
		KnownFiles: map[string]bool{
			"main.rs":    true,
			"helpers.rs": true,
		},
	}
	got := ingestrust.ResolveImport("mod:helpers", ctx)
	if got != "path:./helpers.rs" {
		t.Fatalf("mod:helpers → %q", got)
	}
	got = ingestrust.ResolveImport("crate::helpers", ctx)
	if got != "path:./helpers.rs" {
		t.Fatalf("crate::helpers → %q", got)
	}
	got = ingestrust.ResolveImport("helpers", ctx)
	if got != "path:./helpers.rs" {
		t.Fatalf("helpers → %q", got)
	}
	got = ingestrust.ResolveImport("std::fmt", ctx)
	if got != "rust:std::fmt" {
		t.Fatalf("std::fmt → %q", got)
	}
}

func TestGrains(t *testing.T) {
	lat, ok := ingest.LatticeForFamily("rust")
	if !ok {
		t.Fatal("no lattice")
	}
	grains := lat.Grains()
	want := map[ingest.MoveGrain]bool{
		ingest.MoveGrainAtom: true, ingest.MoveGrainModule: true, ingest.MoveGrainPackage: true,
	}
	for _, g := range grains {
		delete(want, g)
	}
	if len(want) != 0 {
		t.Fatalf("missing grains: %v (have %v)", want, grains)
	}
}
