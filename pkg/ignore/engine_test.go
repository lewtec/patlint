package ignore_test

import (
	"os"
	"path/filepath"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ignore"
)

func TestIsSkippedDirName(t *testing.T) {
	require.False(t, !ignore.IsSkippedDirName("node_modules") || !ignore.IsSkippedDirName("Vendor"),
		"expected skip")
	require.False(t, ignore.IsSkippedDirName("pkg"),
		"pkg should explore")

}

func TestCollectAndEngine(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := lewpath.New(root, rel).String()
		{
			err := os.MkdirAll(filepath.Dir(p), 0o755)
			require.NoError(t, err)
		}

		err := os.WriteFile(p, []byte(body), 0o644)
		require.NoError(t, err)

	}
	write(".gitattributes", ""+
		"*.pb.go linguist-generated=true\n"+
		"gen/** linguist-generated\n"+
		"gen/keep.go -linguist-generated\n")
	write("api/foo.pb.go", "package api\n")
	write("api/hand.go", "package api\n")
	write("gen/a.go", "package gen\n")
	write("gen/keep.go", "package gen\n")
	write("node_modules/x/y.go", "package x\n")

	eng, err := ignore.Collect(t.Context(), root)
	require.NoError(t, err)
	require.Len(t, eng.Sources, 1,
		"sources=%v", eng.Sources)
	// builtins + 3 attr rules
	require.GreaterOrEqual(t, len(eng.Rules), 3+len(ignore.DefaultSkippedDirNames()))

	// Collected patterns include gitignore-style displays
	var sawPB, sawKeep bool
	for _, r := range eng.Rules {
		if r.Kind == ignore.KindLinguistGenerated && r.Display() == "*.pb.go" {
			sawPB = true
		}
		if r.Kind == ignore.KindLinguistGenerated && r.Display() == "!gen/keep.go" {
			sawKeep = true
		}
	}
	require.False(t, !sawPB || !sawKeep,
		"missing patterns in %#v", eng.Rules)

	mustSkip := func(rel string) {
		t.Helper()
		d := eng.Check(lewpath.New(root, rel).String())
		require.False(t, d.Explore,
			"%s: want skip, got explore (pattern=%q)", rel, d.Pattern)

	}
	mustOK := func(rel string) {
		t.Helper()
		d := eng.Check(lewpath.New(root, rel).String())
		require.True(t, d.Explore,
			"%s: want explore, got skip pattern=%q", rel, d.Pattern)

	}

	mustSkip("api/foo.pb.go")
	mustOK("api/hand.go")
	mustSkip("gen/a.go")
	mustOK("gen/keep.go")
	mustSkip("node_modules/x/y.go")
	mustSkip("node_modules")
	require.True(t, eng.SkipDir(lewpath.New(root, "node_modules").String()),
		"expected SkipDir node_modules")
	// gen/ has a negate child — must not skip the whole dir
	require.False(t, eng.SkipDir(lewpath.New(root, "gen").String()),
		"must enter gen/ because of !gen/keep.go")

}

func TestNestedAttributes(t *testing.T) {
	root := t.TempDir()
	{
		err := os.MkdirAll(lewpath.New(root, "pkg").String(), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(root, "pkg", ".gitattributes").String(), []byte("*.gen.go linguist-generated\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(root, "pkg", "a.gen.go").String(), []byte("package p\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(root, "pkg", "a.go").String(), []byte("package p\n"), 0o644)
		require.NoError(t, err)
	}

	eng, err := ignore.Collect(t.Context(), root)
	require.NoError(t, err)
	require.False(t, eng.Explore(lewpath.New(root, "pkg", "a.gen.go").String()),
		"expected generated skip")
	require.True(t, eng.Explore(lewpath.New(root, "pkg", "a.go").String()),
		"expected explore")

}

func TestDoublestar(t *testing.T) {
	root := t.TempDir()
	{
		err := os.WriteFile(lewpath.New(root, ".gitattributes").String(), []byte("**/generated/** linguist-generated\n"), 0o644)
		require.NoError(t, err)
	}

	path := lewpath.New(root, "a", "generated", "b", "c.go").String()
	{
		err := os.MkdirAll(filepath.Dir(path), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(path, []byte("x"), 0o644)
		require.NoError(t, err)
	}

	eng, err := ignore.Collect(t.Context(), root)
	require.NoError(t, err)
	require.False(t, eng.Explore(path),
		"want skip for **/generated/**")

}

func TestGitSkipDir_vsSkipDir(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := lewpath.New(root, rel).String()
		{
			err := os.MkdirAll(filepath.Dir(p), 0o755)
			require.NoError(t, err)
		}

		err := os.WriteFile(p, []byte(body), 0o644)
		require.NoError(t, err)

	}
	write(".gitignore", "secret/\n")
	write(".gitattributes", "/fixtures/** refactree-ignored\n")
	write("secret/x.go", "package s\n")
	write("fixtures/a.go", "package f\n")
	write("keep/a.go", "package k\n")

	eng, err := ignore.Collect(t.Context(), root)
	require.NoError(t, err)

	secret := lewpath.New(root, "secret").String()
	fix := lewpath.New(root, "fixtures").String()
	keep := lewpath.New(root, "keep").String()
	require.False(t, !eng.SkipDir(secret) || !eng.GitSkipDir(secret),
		"gitignore dir: SkipDir and GitSkipDir")
	require.True(t, eng.SkipDir(fix),
		"refactree-ignored: product SkipDir")
	require.False(t, eng.GitSkipDir(fix),
		"refactree-ignored: GitSkipDir must not skip")
	require.False(t, eng.SkipDir(keep) || eng.GitSkipDir(keep),
		"plain dir stays open")

	d := eng.CheckPath(fix, true)
	require.Equal(t, ignore.KindRefactreeIgnored, d.Kind,
		"fixtures kind=%q", d.Kind)

}

func TestRefactreeIgnored(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := lewpath.New(root, rel).String()
		{
			err := os.MkdirAll(filepath.Dir(p), 0o755)
			require.NoError(t, err)
		}

		err := os.WriteFile(p, []byte(body), 0o644)
		require.NoError(t, err)

	}
	write(".gitattributes", ""+
		"vendor-shim/** refactree-ignored\n"+
		"vendor-shim/keep.go -refactree-ignored\n"+
		"*.snap refactree-ignored=true\n"+
		"hand.go refactree-ignored=false\n")
	write("vendor-shim/a.go", "package v\n")
	write("vendor-shim/keep.go", "package v\n")
	write("x.snap", "snapshot\n")
	write("hand.go", "package main\n")
	write("other.go", "package main\n")

	eng, err := ignore.Collect(t.Context(), root)
	require.NoError(t, err)

	var sawIgnore, sawKeep bool
	for _, r := range eng.Rules {
		if r.Kind != ignore.KindRefactreeIgnored {
			continue
		}
		if r.Display() == "vendor-shim/**" {
			sawIgnore = true
		}
		if r.Display() == "!vendor-shim/keep.go" {
			sawKeep = true
		}
	}
	require.False(t, !sawIgnore || !sawKeep,
		"missing refactree-ignored rules in %#v", eng.Rules)
	require.False(t, eng.Explore(lewpath.New(root, "vendor-shim", "a.go").String()),
		"vendor-shim/a.go should be skipped")
	require.True(t, eng.Explore(lewpath.New(root, "vendor-shim", "keep.go").String()),
		"vendor-shim/keep.go should explore via -refactree-ignored")
	require.False(t, eng.Explore(lewpath.New(root, "x.snap").String()),
		"*.snap should be skipped")
	require.True(t, eng.Explore(lewpath.New(root, "hand.go").String()),
		"hand.go should explore (refactree-ignored=false)")
	require.True(t, eng.Explore(lewpath.New(root, "other.go").String()),
		"other.go should explore")
	// Negate under vendor-shim → must still enter the directory.
	require.False(t, eng.SkipDir(lewpath.New(root, "vendor-shim").String()),
		"must enter vendor-shim/ because of !vendor-shim/keep.go")

}

func TestRuleDisplay(t *testing.T) {
	r := ignore.Rule{Pattern: "foo", Negate: true, DirOnly: true}
	g := r.Display()
	require.Equal(t, "!foo/", g,
		"display=%q", g)

	// ensure String doesn't panic and includes pattern metadata
	s := r.String()
	require.NotEmpty(t, s,
		"Rule.String() empty")

}

func TestGitignorePublicDir(t *testing.T) {
	// Astro-style build outDir: /public/ in root .gitignore
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := lewpath.New(root, rel).String()
		{
			err := os.MkdirAll(filepath.Dir(p), 0o755)
			require.NoError(t, err)
		}

		err := os.WriteFile(p, []byte(body), 0o644)
		require.NoError(t, err)

	}
	write(".gitignore", "# Astro build\n/public/\n.astro/\nnode_modules/\n")
	write("src/page.astro", "---\n---\n")
	write("public/index.html", "<html></html>\n")
	write("public/en/index.html", "<html></html>\n")

	eng, err := ignore.Collect(t.Context(), root)
	require.NoError(t, err)

	var sawPublic bool
	for _, r := range eng.Rules {
		if r.Kind == ignore.KindGitignore && r.Display() == "/public/" {
			sawPublic = true
		}
	}
	require.True(t, sawPublic,
		"missing /public/ rule in %#v", eng.Rules)
	require.False(t, eng.Explore(lewpath.New(root, "public", "index.html").String()),
		"public/index.html should be ignored")
	require.False(t, eng.Explore(lewpath.New(root, "public", "en", "index.html").String()),
		"public/en/index.html should be ignored")
	require.True(t, eng.SkipDir(lewpath.New(root, "public").String()),
		"expected SkipDir public/")
	require.True(t, eng.Explore(lewpath.New(root, "src", "page.astro").String()),
		"src/page.astro should explore")

}

func TestGitignoreNegate(t *testing.T) {
	root := t.TempDir()
	{
		err := os.WriteFile(lewpath.New(root, ".gitignore").String(), []byte("build/\n!build/keep.go\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.MkdirAll(lewpath.New(root, "build").String(), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(root, "build", "a.go").String(), []byte("package b\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(root, "build", "keep.go").String(), []byte("package b\n"), 0o644)
		require.NoError(t, err)
	}

	eng, err := ignore.Collect(t.Context(), root)
	require.NoError(t, err)
	require.False(t, eng.Explore(lewpath.New(root, "build", "a.go").String()),
		"build/a.go should skip")
	require.True(t, eng.Explore(lewpath.New(root, "build", "keep.go").String()),
		"build/keep.go should explore via !")
	require.False(t, eng.SkipDir(lewpath.New(root, "build").String()),
		"must enter build/ for negate child")

}

func TestNestedGitignore(t *testing.T) {
	root := t.TempDir()
	{
		err := os.MkdirAll(lewpath.New(root, "pkg").String(), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(root, ".gitignore").String(), []byte("*.tmp\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(root, "pkg", ".gitignore").String(), []byte("!keep.tmp\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(root, "a.tmp").String(), []byte("x"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(root, "pkg", "keep.tmp").String(), []byte("x"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(root, "pkg", "drop.tmp").String(), []byte("x"), 0o644)
		require.NoError(t, err)
	}

	eng, err := ignore.Collect(t.Context(), root)
	require.NoError(t, err)
	require.False(t, eng.Explore(lewpath.New(root, "a.tmp").String()),
		"root a.tmp should skip")
	require.True(t, eng.Explore(lewpath.New(root, "pkg", "keep.tmp").String()),
		"pkg/keep.tmp should explore via nested !")
	require.False(t, eng.Explore(lewpath.New(root, "pkg", "drop.tmp").String()),
		"pkg/drop.tmp still matches *.tmp")

}
