package js_test

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest/ecma/js"
	"github.com/lewtec/patlint/pkg/pattern"
)

func TestEmptySFCShellFromPackEmbeds(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	p := vm
	astro := js.EmptySFCShell(p, "Page.astro", "export const x = 1")
	if !strings.HasPrefix(astro, "---\n") || !strings.Contains(astro, "---\nexport const x = 1") {
		t.Fatalf("astro shell=%q", astro)
	}
	vue := js.EmptySFCShell(p, "App.vue", "export const x = 1")
	if !strings.Contains(vue, "<script>") || strings.HasPrefix(vue, "---") {
		t.Fatalf("vue shell=%q", vue)
	}
	plain := js.EmptySFCShell(p, "mod.ts", "export const x = 1")
	if strings.Contains(plain, "<script>") || strings.HasPrefix(plain, "---") {
		t.Fatalf("ts shell=%q", plain)
	}
}
