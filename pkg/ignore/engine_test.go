package ignore_test

import (
	"os"
	"path/filepath"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ignore"
)

func TestIsSkippedDirName(t *testing.T) {
	if !ignore.IsSkippedDirName("node_modules") || !ignore.IsSkippedDirName("Vendor") {
		t.Fatal("expected skip")
	}
	if ignore.IsSkippedDirName("pkg") {
		t.Fatal("pkg should explore")
	}
}

func TestCollectAndEngine(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := lewpath.New(root, rel).String()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
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
	if err != nil {
		t.Fatal(err)
	}
	if len(eng.Sources) != 1 {
		t.Fatalf("sources=%v", eng.Sources)
	}
	// builtins + 3 attr rules
	if len(eng.Rules) < 3+len(ignore.DefaultSkippedDirNames()) {
		t.Fatalf("rules=%d", len(eng.Rules))
	}
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
	if !sawPB || !sawKeep {
		t.Fatalf("missing patterns in %#v", eng.Rules)
	}

	mustSkip := func(rel string) {
		t.Helper()
		d := eng.Check(lewpath.New(root, rel).String())
		if d.Explore {
			t.Fatalf("%s: want skip, got explore (pattern=%q)", rel, d.Pattern)
		}
	}
	mustOK := func(rel string) {
		t.Helper()
		d := eng.Check(lewpath.New(root, rel).String())
		if !d.Explore {
			t.Fatalf("%s: want explore, got skip pattern=%q", rel, d.Pattern)
		}
	}

	mustSkip("api/foo.pb.go")
	mustOK("api/hand.go")
	mustSkip("gen/a.go")
	mustOK("gen/keep.go")
	mustSkip("node_modules/x/y.go")
	mustSkip("node_modules")

	if !eng.SkipDir(lewpath.New(root, "node_modules").String()) {
		t.Fatal("expected SkipDir node_modules")
	}
	// gen/ has a negate child — must not skip the whole dir
	if eng.SkipDir(lewpath.New(root, "gen").String()) {
		t.Fatal("must enter gen/ because of !gen/keep.go")
	}
}

func TestNestedAttributes(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(lewpath.New(root, "pkg").String(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "pkg", ".gitattributes").String(), []byte("*.gen.go linguist-generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "pkg", "a.gen.go").String(), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "pkg", "a.go").String(), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	eng, err := ignore.Collect(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if eng.Explore(lewpath.New(root, "pkg", "a.gen.go").String()) {
		t.Fatal("expected generated skip")
	}
	if !eng.Explore(lewpath.New(root, "pkg", "a.go").String()) {
		t.Fatal("expected explore")
	}
}

func TestDoublestar(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(lewpath.New(root, ".gitattributes").String(), []byte("**/generated/** linguist-generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := lewpath.New(root, "a", "generated", "b", "c.go").String()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	eng, err := ignore.Collect(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if eng.Explore(path) {
		t.Fatal("want skip for **/generated/**")
	}
}

func TestGitSkipDir_vsSkipDir(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := lewpath.New(root, rel).String()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "secret/\n")
	write(".gitattributes", "/fixtures/** refactree-ignored\n")
	write("secret/x.go", "package s\n")
	write("fixtures/a.go", "package f\n")
	write("keep/a.go", "package k\n")

	eng, err := ignore.Collect(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	secret := lewpath.New(root, "secret").String()
	fix := lewpath.New(root, "fixtures").String()
	keep := lewpath.New(root, "keep").String()
	if !eng.SkipDir(secret) || !eng.GitSkipDir(secret) {
		t.Fatal("gitignore dir: SkipDir and GitSkipDir")
	}
	if !eng.SkipDir(fix) {
		t.Fatal("refactree-ignored: product SkipDir")
	}
	if eng.GitSkipDir(fix) {
		t.Fatal("refactree-ignored: GitSkipDir must not skip")
	}
	if eng.SkipDir(keep) || eng.GitSkipDir(keep) {
		t.Fatal("plain dir stays open")
	}
	d := eng.CheckPath(fix, true)
	if d.Kind != ignore.KindRefactreeIgnored {
		t.Fatalf("fixtures kind=%q", d.Kind)
	}
}

func TestRefactreeIgnored(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := lewpath.New(root, rel).String()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
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
	if err != nil {
		t.Fatal(err)
	}
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
	if !sawIgnore || !sawKeep {
		t.Fatalf("missing refactree-ignored rules in %#v", eng.Rules)
	}

	if eng.Explore(lewpath.New(root, "vendor-shim", "a.go").String()) {
		t.Fatal("vendor-shim/a.go should be skipped")
	}
	if !eng.Explore(lewpath.New(root, "vendor-shim", "keep.go").String()) {
		t.Fatal("vendor-shim/keep.go should explore via -refactree-ignored")
	}
	if eng.Explore(lewpath.New(root, "x.snap").String()) {
		t.Fatal("*.snap should be skipped")
	}
	if !eng.Explore(lewpath.New(root, "hand.go").String()) {
		t.Fatal("hand.go should explore (refactree-ignored=false)")
	}
	if !eng.Explore(lewpath.New(root, "other.go").String()) {
		t.Fatal("other.go should explore")
	}
	// Negate under vendor-shim → must still enter the directory.
	if eng.SkipDir(lewpath.New(root, "vendor-shim").String()) {
		t.Fatal("must enter vendor-shim/ because of !vendor-shim/keep.go")
	}
}

func TestRuleDisplay(t *testing.T) {
	r := ignore.Rule{Pattern: "foo", Negate: true, DirOnly: true}
	if g := r.Display(); g != "!foo/" {
		t.Fatalf("display=%q", g)
	}
	// ensure String doesn't panic and includes pattern metadata
	if s := r.String(); s == "" {
		t.Fatal("Rule.String() empty")
	}
}

func TestGitignorePublicDir(t *testing.T) {
	// Astro-style build outDir: /public/ in root .gitignore
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := lewpath.New(root, rel).String()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "# Astro build\n/public/\n.astro/\nnode_modules/\n")
	write("src/page.astro", "---\n---\n")
	write("public/index.html", "<html></html>\n")
	write("public/en/index.html", "<html></html>\n")

	eng, err := ignore.Collect(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	var sawPublic bool
	for _, r := range eng.Rules {
		if r.Kind == ignore.KindGitignore && r.Display() == "/public/" {
			sawPublic = true
		}
	}
	if !sawPublic {
		t.Fatalf("missing /public/ rule in %#v", eng.Rules)
	}

	if eng.Explore(lewpath.New(root, "public", "index.html").String()) {
		t.Fatal("public/index.html should be ignored")
	}
	if eng.Explore(lewpath.New(root, "public", "en", "index.html").String()) {
		t.Fatal("public/en/index.html should be ignored")
	}
	if !eng.SkipDir(lewpath.New(root, "public").String()) {
		t.Fatal("expected SkipDir public/")
	}
	if !eng.Explore(lewpath.New(root, "src", "page.astro").String()) {
		t.Fatal("src/page.astro should explore")
	}
}

func TestGitignoreNegate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(lewpath.New(root, ".gitignore").String(), []byte("build/\n!build/keep.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lewpath.New(root, "build").String(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "build", "a.go").String(), []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "build", "keep.go").String(), []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eng, err := ignore.Collect(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if eng.Explore(lewpath.New(root, "build", "a.go").String()) {
		t.Fatal("build/a.go should skip")
	}
	if !eng.Explore(lewpath.New(root, "build", "keep.go").String()) {
		t.Fatal("build/keep.go should explore via !")
	}
	if eng.SkipDir(lewpath.New(root, "build").String()) {
		t.Fatal("must enter build/ for negate child")
	}
}

func TestNestedGitignore(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(lewpath.New(root, "pkg").String(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, ".gitignore").String(), []byte("*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "pkg", ".gitignore").String(), []byte("!keep.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "a.tmp").String(), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "pkg", "keep.tmp").String(), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lewpath.New(root, "pkg", "drop.tmp").String(), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	eng, err := ignore.Collect(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if eng.Explore(lewpath.New(root, "a.tmp").String()) {
		t.Fatal("root a.tmp should skip")
	}
	if !eng.Explore(lewpath.New(root, "pkg", "keep.tmp").String()) {
		t.Fatal("pkg/keep.tmp should explore via nested !")
	}
	if eng.Explore(lewpath.New(root, "pkg", "drop.tmp").String()) {
		t.Fatal("pkg/drop.tmp still matches *.tmp")
	}
}
