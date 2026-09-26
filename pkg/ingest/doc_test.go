package ingest_test

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestDocFor_PythonFunction(t *testing.T) {
	dir := t.TempDir()
	content := "def helper(x):\n    \"\"\"does help\"\"\"\n    return x\n"
	{
		err := os.WriteFile(lewpath.New(dir, "helper.py").String(), []byte(content), 0644)
		require.NoError(t, err)
	}

	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	doc, err := w.Doc(t.Context(), dir, "path:./helper.py::helper")
	require.NoError(t, err,
		"doc lookup failed: %v", err)
	require.Equal(t, "helper", doc.Name,
		"unexpected Name: %q", doc.Name)
	require.True(t, strings.Contains(doc.Signature, "def helper(x)"),
		"unexpected signature: %q", doc.Signature)
	require.Equal(t, "does help", doc.DocString,
		"unexpected docstring: %q", doc.DocString)

}

func TestDocFor_PythonClass(t *testing.T) {
	dir := t.TempDir()
	content := "class Greeter:\n    \"\"\"Greeter docs\"\"\"\n\n    def hi(self):\n        return 'hi'\n"
	{
		err := os.WriteFile(lewpath.New(dir, "helper.py").String(), []byte(content), 0644)
		require.NoError(t, err)
	}

	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	doc, err := w.Doc(t.Context(), dir, "path:./helper.py::Greeter")
	require.NoError(t, err,
		"doc lookup failed: %v", err)
	require.Equal(t, "Greeter", doc.Name,
		"unexpected Name: %q", doc.Name)
	require.True(t, strings.Contains(doc.Signature, "class Greeter"),
		"unexpected signature: %q", doc.Signature)
	require.Equal(t, "Greeter docs", doc.DocString,
		"unexpected docstring: %q", doc.DocString)

}

func TestDocFor_DirectoryReference_PythonRejected(t *testing.T) {
	dir := t.TempDir()
	{
		err := os.MkdirAll(lewpath.New(dir, "pkg").String(), 0755)
		require.NoError(t, err)
	}

	content := "def helper():\n    \"\"\"from init\"\"\"\n    pass\n"
	{
		err := os.WriteFile(lewpath.New(dir, "pkg", "__init__.py").String(), []byte(content), 0644)
		require.NoError(t, err)
	}

	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	_, err = w.Doc(t.Context(), dir, "path:./pkg::helper")
	require.Error(t, err,
		"expected error for directory reference")
	require.ErrorIs(t, err, ingest.ErrDirectorySymbolUnsupported,
		"unexpected error: %v", err)

}

func TestDocFor_DirectoryReference_JSRejected(t *testing.T) {
	dir := t.TempDir()
	{
		err := os.MkdirAll(lewpath.New(dir, "pkg").String(), 0755)
		require.NoError(t, err)
	}

	content := "// helper docs\nfunction helper() {\n}\nexport { helper };\n"
	{
		err := os.WriteFile(lewpath.New(dir, "pkg", "index.js").String(), []byte(content), 0644)
		require.NoError(t, err)
	}

	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	_, err = w.Doc(t.Context(), dir, "path:./pkg::helper")
	require.Error(t, err,
		"expected error for directory reference")
	require.ErrorIs(t, err, ingest.ErrDirectorySymbolUnsupported,
		"unexpected error: %v", err)

}

func TestDocFor_DirectoryReference_GoFiles(t *testing.T) {
	dir := t.TempDir()
	{
		err := os.MkdirAll(lewpath.New(dir, "pkg").String(), 0755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(dir, "pkg", "a.go").String(), []byte("package pkg\n\n// helper docs\nfunc helper() {\n}\n"), 0644)
		require.NoError(t, err)
	}

	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	doc, err := w.Doc(t.Context(), dir, "path:./pkg::helper")
	require.NoError(t, err,
		"doc lookup failed: %v", err)
	require.Equal(t, "helper", doc.Name,
		"unexpected Name: %q", doc.Name)
	require.True(t, strings.Contains(doc.DocString, "helper docs"),
		"unexpected docstring: %q", doc.DocString)

}

func TestDocFor_GoProviderStdlibFunction(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	doc, err := w.Doc(t.Context(), ".", "go:fmt::Printf")
	require.NoError(t, err,
		"doc lookup failed: %v", err)
	require.Equal(t, "Printf", doc.Name,
		"unexpected Name: %q", doc.Name)
	require.True(t, strings.Contains(doc.Signature, "func Printf("),
		"unexpected signature: %q", doc.Signature)
	require.True(t, strings.Contains(doc.DocString, "Printf formats according"),
		"unexpected docstring: %q", doc.DocString)

}
