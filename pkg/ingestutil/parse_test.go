package ingestutil

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestParseSourceFile(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := ParseSourceFile(ccgo.Engine{}, path, "go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	if string(pf.Source) != "package p\n" {
		t.Fatalf("source: %q", pf.Source)
	}
	if pf.Root == nil {
		t.Fatal("nil root")
	}
	pf.Close() // double-close must be safe
}

func TestParseSourceFileEmptyLanguage(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.go").String()
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSourceFile(ccgo.Engine{}, path, ""); err == nil {
		t.Fatal("expected empty language error")
	}
}

func TestParseSourceFileUnknownLanguage(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "x.unknownlang").String()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSourceFile(ccgo.Engine{}, path, "not-a-grammar"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseSource(t *testing.T) {
	content := []byte("package p\n")
	pf, err := ParseSource(ccgo.Engine{}, content, "x.go", "go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	if pf.Root == nil || string(pf.Source) != string(content) {
		t.Fatal("bad parse")
	}
}
