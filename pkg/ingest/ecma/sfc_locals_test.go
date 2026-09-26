package ecma_test

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/ecma/js"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/walker"
)

func TestVueScriptAbstractHoles(t *testing.T) {
	src := `<script>
  function maxAb(a, b) {
    if (a > b) { return a; }
    return b;
  }
  const maxXy = (x, y) => {
    if (x > y) { return x; }
    return y;
  };
</script>
<template><div /></template>
`
	wantHoles(t, "vue", "A.vue", src, 8)
}

func TestSvelteScriptAbstractHoles(t *testing.T) {
	src := `<script>
  function maxAb(a, b) {
    if (a > b) { return a; }
    return b;
  }
  const maxXy = (x, y) => {
    if (x > y) { return x; }
    return y;
  };
</script>
`
	wantHoles(t, "svelte", "A.svelte", src, 8)
}

func TestAstroFrontmatterAbstractHoles(t *testing.T) {
	src := `---
function maxAb(a, b) {
  if (a > b) { return a; }
  return b;
}
const maxXy = (x, y) => {
  if (x > y) { return x; }
  return y;
};
---
<div />
`
	wantHoles(t, "astro", "A.astro", src, 3)
}

func wantHoles(t *testing.T, grammarName, path, src string, minStart uint32) {
	t.Helper()
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, []byte(src), path, grammarName)
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	units, err := w.AbstractFile(t.Context(), pf.Root, pf.Source, path, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 2 {
		t.Fatalf("units=%d %#v", len(units), units)
	}
	for _, u := range units {
		got := ingest.FormatTerms(u.Terms)
		if !strings.Contains(got, "%r1") || !strings.Contains(got, "%r2") {
			t.Fatalf("%s: %s", u.Name, got)
		}
		if u.Span.StartByte < minStart || int(u.Span.EndByte) > len(src) {
			t.Fatalf("span out of host file: %v", u.Span)
		}
	}
}
