package ecma_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/ecma/js"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestVueIngestScriptSymbols(t *testing.T) {
	dir := t.TempDir()
	app := `<script lang="ts">
import SearchBar from './SearchBar.vue';
export let initialQuery = '';
let query = initialQuery;
function handleSearch(event: Event) {
  event.preventDefault();
}
</script>
<template>
  <form @submit="handleSearch">
    <input v-model="query" />
    <SearchBar />
    <span>{{ query }}</span>
  </form>
</template>
`
	if err := os.WriteFile(lewpath.New(dir, "App.vue").String(), []byte(app), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "SearchBar.vue").String(), []byte(
		`<script></script><template><div/></template>`,
	), 0o644); err != nil {
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
	var appFile bool
	for _, f := range result.Files {
		if f.Path == "App.vue" && f.Language == "vue" {
			appFile = true
		}
	}
	if !appFile {
		t.Fatalf("files=%#v", result.Files)
	}
	names := map[string]bool{}
	for _, e := range result.Atoms {
		ref := ingest.ParseReference(e.Reference)
		if strings.Contains(ref.Path, "App.vue") {
			names[ref.Name] = true
		}
	}
	for _, want := range []string{"initialQuery", "query", "handleSearch"} {
		if !names[want] {
			t.Fatalf("missing entity %q in %#v", want, names)
		}
	}
	usageNames := map[string]bool{}
	usagePaths := map[string]bool{}
	for _, r := range result.Uses {
		tgt := ingest.ParseReference(r.Target)
		if tgt.Name != "" {
			usageNames[tgt.Name] = true
		}
		if tgt.Path != "" {
			usagePaths[filepath.Base(tgt.Path)] = true
		}
	}
	for _, want := range []string{"handleSearch", "query"} {
		if !usageNames[want] {
			t.Fatalf("missing template relation to %q", want)
		}
	}
	if !usagePaths["SearchBar.vue"] {
		t.Fatal("missing component relation to SearchBar.vue")
	}
}

func TestSvelteIngestScriptSymbols(t *testing.T) {
	dir := t.TempDir()
	src := `<script lang="ts">
import { Search } from 'lucide-svelte';
export let initialQuery = '';
let query = initialQuery;
function handleSearch(event: Event) {
  event.preventDefault();
}
</script>
<form on:submit={handleSearch}>
<input bind:value={query} />
<Search size={18} />
</form>
`
	if err := os.WriteFile(lewpath.New(dir, "SearchBar.svelte").String(), []byte(src), 0o644); err != nil {
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
	if len(result.Files) != 1 || result.Files[0].Language != "svelte" {
		t.Fatalf("files=%#v", result.Files)
	}
	names := map[string]bool{}
	for _, e := range result.Atoms {
		names[ingest.ParseReference(e.Reference).Name] = true
	}
	for _, want := range []string{"initialQuery", "query", "handleSearch"} {
		if !names[want] {
			t.Fatalf("missing entity %q in %#v", want, names)
		}
	}
	usageTargets := map[string]bool{}
	for _, r := range result.Uses {
		usageTargets[ingest.ParseReference(r.Target).Name] = true
	}
	for _, want := range []string{"handleSearch", "query", "Search"} {
		if !usageTargets[want] {
			t.Fatalf("missing markup relation to %q", want)
		}
	}
}

func TestAstroIngestFrontmatterSymbols(t *testing.T) {
	dir := t.TempDir()
	page := `---
import Header from './Header.astro';
const title = 'Hi';
function greet() {
  return title;
}
---
<html>
  <Header />
  <h1>{title}</h1>
  <button onclick={greet}>go</button>
</html>
`
	if err := os.WriteFile(lewpath.New(dir, "Page.astro").String(), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "Header.astro").String(), []byte("---\n---\n<div/>\n"), 0o644); err != nil {
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
	names := map[string]bool{}
	for _, e := range result.Atoms {
		ref := ingest.ParseReference(e.Reference)
		if strings.Contains(ref.Path, "Page.astro") {
			names[ref.Name] = true
		}
	}
	for _, want := range []string{"title", "greet"} {
		if !names[want] {
			t.Fatalf("missing entity %q in %#v", want, names)
		}
	}
	usageNames := map[string]bool{}
	usagePaths := map[string]bool{}
	for _, r := range result.Uses {
		tgt := ingest.ParseReference(r.Target)
		if tgt.Name != "" {
			usageNames[tgt.Name] = true
		}
		if tgt.Path != "" {
			usagePaths[filepath.Base(tgt.Path)] = true
		}
	}
	for _, want := range []string{"title", "greet"} {
		if !usageNames[want] {
			t.Fatalf("missing markup relation to %q", want)
		}
	}
	if !usagePaths["Header.astro"] {
		t.Fatal("missing component relation to Header.astro")
	}
}

func TestEmbedHostLanguageForFile(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ file, want string }{
		{"X.vue", "vue"},
		{"X.svelte", "svelte"},
		{"X.astro", "astro"},
	} {
		lang, ok := vm.HostLanguage(tc.file)
		if !ok || lang != tc.want {
			t.Fatalf("%s: got %q ok=%v", tc.file, lang, ok)
		}
		if !vm.PathHasEmbeds(tc.file) {
			t.Fatalf("%s: pack should declare embeds", tc.file)
		}
	}
	if vm.PathHasEmbeds("main.js") {
		t.Fatal("plain js must not declare embeds")
	}
}
