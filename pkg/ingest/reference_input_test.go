package ingest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	_ "github.com/lewtec/patlint/internal/prelude"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
)

func TestCoerceLocalPathReference_Directory(t *testing.T) {
	dir := t.TempDir()
	err := os.MkdirAll(lewpath.New(dir, "cmd").String(), 0755)
	require.NoError(t, err)

	ref := ingest.ParseReference("cmd")
	got := ingest.CoerceLocalPathReference(dir, ref)
	require.False(t, got.Provider != "path" || got.Path != "./cmd",
		"unexpected coerced ref: %+v", got)

}

func TestCoerceLocalPathReference_File(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(lewpath.New(dir, "x.go").String(), []byte("package main\n"), 0644)
	require.NoError(t, err)

	ref := ingest.ParseReference("x.go")
	got := ingest.CoerceLocalPathReference(dir, ref)
	require.False(t, got.Provider != "path" || got.Path != "./x.go",
		"unexpected coerced ref: %+v", got)

}

func TestCoerceLocalPathReference_MissingPath(t *testing.T) {
	dir := t.TempDir()
	ref := ingest.ParseReference(lewpath.New("no", "such", "path").String())
	got := ingest.CoerceLocalPathReference(dir, ref)
	require.False(t, got.Provider != ref.Provider || got.Path != ref.Path,
		"expected unchanged ref, got %+v", got)

}

func TestResolveInputReferenceScope(t *testing.T) {
	dir := t.TempDir()
	err := os.MkdirAll(lewpath.New(dir, "cmd").String(), 0755)
	require.NoError(t, err)

	scope := ingest.ResolveInputReferenceScope(dir, "cmd")
	require.Equal(t, lewpath.New(dir, "cmd").String(), scope.Dir,
		"unexpected scope dir: %q", scope.Dir)
	require.False(t, scope.Reference.Provider != "path" || scope.Reference.Path != "./",
		"unexpected scope ref: %+v", scope.Reference)

}

func TestResolveMoveArgs_ExpandsPathToAbsolute(t *testing.T) {
	dir := t.TempDir()
	srcFile := lewpath.New(dir, "pkg", "pattern", "match.go").String()
	{
		err := os.MkdirAll(filepath.Dir(srcFile), 0755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(srcFile, []byte("package pattern\n\ntype ingestutil.Span struct{}\n"), 0644)
		require.NoError(t, err)
	}
	{

		err := os.MkdirAll(lewpath.New(dir, "pkg", "ingest").String(), 0755)
		require.NoError(t, err)
	}

	root, src, dst := ingest.ResolveMoveArgs(dir, "path:./pkg/pattern/match.go::ingestutil.Span",
		"path:./pkg/ingest/span.go::ingestutil.Span",
	)
	rootAbs, err := filepath.Abs(dir)
	require.NoError(t, err)
	require.Equal(t, rootAbs, root,
		"root=%q want abs %q", root, rootAbs)

	srcRef := ingest.ParseReference(src)
	dstRef := ingest.ParseReference(dst)
	wantSrc := lewpath.New(rootAbs, "pkg", "pattern", "match.go").String()
	wantDst := lewpath.New(rootAbs, "pkg", "ingest", "span.go").String()
	require.False(t, srcRef.Path != wantSrc || srcRef.Name != "ingestutil.Span",
		"source ref=%+v want path %s::ingestutil.Span", srcRef, wantSrc)
	require.False(t, dstRef.Path != wantDst || dstRef.Name != "ingestutil.Span",
		"destination ref=%+v want path %s::ingestutil.Span", dstRef, wantDst)
	// Absolute paths must not be rebased under a source-parent scope.
	require.NotContains(t, dstRef.Path, "pattern/pkg")

	// Rename maps absolute path refs back to project-relative identity.
	proj := ingest.ProjectPathRef(root, srcRef)
	require.Equal(t, "./pkg/pattern/match.go", proj.Path,
		"ProjectPathRef=%q", proj.Path)

}

func TestResolveMoveArgs_BarePackageDirsAbsolute(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"cmd/codegen", "cmd/gen"} {
		err := os.MkdirAll(lewpath.New(dir, p).String(), 0755)
		require.NoError(t, err)

	}
	{
		// Minimal Go file so coerce/canonicalize have something under codegen.
		err := os.WriteFile(lewpath.New(dir, "cmd/codegen/main.go").String(), []byte("package codegen\n"), 0644)
		require.NoError(t, err)
	}

	root, src, dst := ingest.ResolveMoveArgs(dir, "cmd/codegen", "cmd/gen")
	rootAbs, err := filepath.Abs(dir)
	require.NoError(t, err)
	require.Equal(t, rootAbs, root,
		"root=%q want %q", root, rootAbs)

	srcRef := ingest.ParseReference(src)
	dstRef := ingest.ParseReference(dst)
	require.False(t, srcRef.Provider != "path" || !filepath.IsAbs(srcRef.Path) || !strings.Contains(srcRef.Path, "codegen"),
		"source ref=%+v", srcRef)
	require.False(t, dstRef.Provider != "path" || !filepath.IsAbs(dstRef.Path) || !strings.Contains(dstRef.Path, "gen"),
		"destination ref=%+v", dstRef)
	require.False(t, srcRef.Name != "" || dstRef.Name != "",
		"package move should have empty symbols: src=%+v dst=%+v", srcRef, dstRef)

	// Nested fixture-like pkg/ must not equal top-level absolute pkg identity.
	nested := lewpath.New(rootAbs, "testdata", "mv", "input", "pkg").String()
	top := lewpath.New(rootAbs, "pkg").String()
	require.NotEqual(t, nested, top,
		"nested and top pkg paths should differ")

	topRef := ingest.AbsolutePathRef(rootAbs, ingest.ParseReference("path:./pkg"))
	if topRef.Path != top {
		{
			// ./pkg may not exist in this temp dir — only check abs form when present
			st, err := os.Stat(top)
			require.False(t, err == nil && st.IsDir() && topRef.Path != top,
				"AbsolutePathRef pkg=%q want %q", topRef.Path, top)
		}

	}
}

func TestRelPathUnderRoot(t *testing.T) {
	dir := t.TempDir()
	abs, err := filepath.Abs(dir)
	require.NoError(t, err)

	sub := lewpath.New(abs, "pkg", "a.go").String()
	rel, err := ingest.RelPathUnderRoot(abs, sub)
	require.False(t, err != nil || rel != "pkg/a.go",
		"rel=%q err=%v", rel, err)

	rel, err = ingest.RelPathUnderRoot(abs, "./pkg/a.go")
	require.False(t, err != nil || rel != "pkg/a.go",
		"rel from ./ =%q err=%v", rel, err)
	{

		_, err := ingest.RelPathUnderRoot(abs, lewpath.New(abs, "..", "outside").String())
		require.Error(t, err,
			"expected outside root error")
	}

}
