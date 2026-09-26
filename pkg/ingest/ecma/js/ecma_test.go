package js_test

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/ecma/js"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestECMAIngestTypeScriptAndTSX(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "types.ts").String(), []byte(`
export interface User {
  id: string
}
export type ID = string
export function greet(name: string): string {
  return name
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "Button.tsx").String(), []byte(`
export default function Button(props: { label: string }) {
  return <button>{props.label}</button>
}
export const variant = "primary"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	result, err := w.Load(t.Context(), ingest.SourceProject(dir), ingest.MaterializeOptions{ExpandImports: true})
	if err != nil {
		t.Fatal(err)
	}

	langs := map[string]string{}
	for _, f := range result.Files {
		langs[strings.TrimPrefix(f.Path, "./")] = f.Language
	}
	if langs["types.ts"] != "typescript" || langs["Button.tsx"] != "tsx" {
		t.Fatalf("expected types.ts=typescript Button.tsx=tsx, got %#v", langs)
	}

	names := map[string]bool{}
	for _, e := range result.Atoms {
		ref := ingest.ParseReference(e.Reference)
		if ref.Name != "" {
			names[ref.Name] = true
		}
	}
	for _, want := range []string{"User", "ID", "greet", "Button", "variant"} {
		if !names[want] {
			t.Fatalf("missing entity %q in %#v (entities=%d files=%d)", want, names, len(result.Atoms), len(result.Files))
		}
	}
}

func TestECMALanguageForExtensions(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ file, want string }{
		{"a.ts", "typescript"},
		{"b.tsx", "tsx"},
		{"c.jsx", "javascript"},
		{"d.js", "javascript"},
		{"e.mjs", "javascript"},
	} {
		lang, ok := vm.HostLanguage(tc.file)
		if !ok || lang != tc.want {
			t.Fatalf("LanguageForFile(%q)=%q ok=%v want %q", tc.file, lang, ok, tc.want)
		}
	}
	if lang, ok := vm.HostLanguage("x.vue"); !ok || lang != "vue" {
		t.Fatalf("vue pack: %q ok=%v", lang, ok)
	}
	if lang, ok := vm.HostLanguage("x.astro"); !ok || lang != "astro" {
		t.Fatalf("astro pack: %q ok=%v", lang, ok)
	}
}
