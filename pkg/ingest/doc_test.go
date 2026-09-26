package ingest_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestDocFor_PythonFunction(t *testing.T) {
	dir := t.TempDir()
	content := "def helper(x):\n    \"\"\"does help\"\"\"\n    return x\n"
	if err := os.WriteFile(lewpath.New(dir, "helper.py").String(), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}

	doc, err := w.Doc(t.Context(), dir, "path:./helper.py::helper")
	if err != nil {
		t.Fatalf("doc lookup failed: %v", err)
	}

	if doc.Name != "helper" {
		t.Fatalf("unexpected Name: %q", doc.Name)
	}
	if !strings.Contains(doc.Signature, "def helper(x)") {
		t.Fatalf("unexpected signature: %q", doc.Signature)
	}
	if doc.DocString != "does help" {
		t.Fatalf("unexpected docstring: %q", doc.DocString)
	}
}

func TestDocFor_PythonClass(t *testing.T) {
	dir := t.TempDir()
	content := "class Greeter:\n    \"\"\"Greeter docs\"\"\"\n\n    def hi(self):\n        return 'hi'\n"
	if err := os.WriteFile(lewpath.New(dir, "helper.py").String(), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}

	doc, err := w.Doc(t.Context(), dir, "path:./helper.py::Greeter")
	if err != nil {
		t.Fatalf("doc lookup failed: %v", err)
	}

	if doc.Name != "Greeter" {
		t.Fatalf("unexpected Name: %q", doc.Name)
	}
	if !strings.Contains(doc.Signature, "class Greeter") {
		t.Fatalf("unexpected signature: %q", doc.Signature)
	}
	if doc.DocString != "Greeter docs" {
		t.Fatalf("unexpected docstring: %q", doc.DocString)
	}
}

func TestDocFor_DirectoryReference_PythonRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(lewpath.New(dir, "pkg").String(), 0755); err != nil {
		t.Fatal(err)
	}
	content := "def helper():\n    \"\"\"from init\"\"\"\n    pass\n"
	if err := os.WriteFile(lewpath.New(dir, "pkg", "__init__.py").String(), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}

	_, err = w.Doc(t.Context(), dir, "path:./pkg::helper")
	if err == nil {
		t.Fatal("expected error for directory reference")
	}
	if !errors.Is(err, ingest.ErrDirectorySymbolUnsupported) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDocFor_DirectoryReference_JSRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(lewpath.New(dir, "pkg").String(), 0755); err != nil {
		t.Fatal(err)
	}
	content := "// helper docs\nfunction helper() {\n}\nexport { helper };\n"
	if err := os.WriteFile(lewpath.New(dir, "pkg", "index.js").String(), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}

	_, err = w.Doc(t.Context(), dir, "path:./pkg::helper")
	if err == nil {
		t.Fatal("expected error for directory reference")
	}
	if !errors.Is(err, ingest.ErrDirectorySymbolUnsupported) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDocFor_DirectoryReference_GoFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(lewpath.New(dir, "pkg").String(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(dir, "pkg", "a.go").String(), []byte("package pkg\n\n// helper docs\nfunc helper() {\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}

	doc, err := w.Doc(t.Context(), dir, "path:./pkg::helper")
	if err != nil {
		t.Fatalf("doc lookup failed: %v", err)
	}
	if doc.Name != "helper" {
		t.Fatalf("unexpected Name: %q", doc.Name)
	}
	if !strings.Contains(doc.DocString, "helper docs") {
		t.Fatalf("unexpected docstring: %q", doc.DocString)
	}
}

func TestDocFor_GoProviderStdlibFunction(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := w.Doc(t.Context(), ".", "go:fmt::Printf")
	if err != nil {
		t.Fatalf("doc lookup failed: %v", err)
	}

	if doc.Name != "Printf" {
		t.Fatalf("unexpected Name: %q", doc.Name)
	}
	if !strings.Contains(doc.Signature, "func Printf(") {
		t.Fatalf("unexpected signature: %q", doc.Signature)
	}
	if !strings.Contains(doc.DocString, "Printf formats according") {
		t.Fatalf("unexpected docstring: %q", doc.DocString)
	}
}
