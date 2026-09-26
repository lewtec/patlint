package pattern

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingestutil"

	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestUnquoteLiteralMap_Escapes(t *testing.T) {
	raw := []byte(`"a\tb\n\\"`)
	content, srcOf, closeOff, ok := unquoteLiteralMap(raw)
	if !ok {
		t.Fatal("unquote failed")
	}
	wantContent := "a\tb\n\\"
	if content != wantContent {
		t.Fatalf("content=%q want %q", content, wantContent)
	}
	if closeOff != len(raw)-1 {
		t.Fatalf("closeOff=%d want %d", closeOff, len(raw)-1)
	}
	if len(srcOf) != len(content) {
		t.Fatalf("srcOf len=%d content len=%d", len(srcOf), len(content))
	}
	if srcOf[0] != 1 {
		t.Fatalf("srcOf[0]=%d want 1", srcOf[0])
	}
	if srcOf[1] != 2 {
		t.Fatalf("srcOf[1]=%d want 2 (start of \\t)", srcOf[1])
	}
	if srcOf[2] != 4 {
		t.Fatalf("srcOf[2]=%d want 4", srcOf[2])
	}
}

func TestContentSpanToSource_IdentAndQuoted(t *testing.T) {
	ident := []byte("TestFoo")
	buf := make([]byte, 100+len(ident))
	copy(buf[100:], ident)
	tk := tok{Span: ingestutil.Span{StartByte: 100, EndByte: 100 + uint32(len(ident))}}
	content, srcOf, closeOff, quoted := tokenContentMap(buf, tk)
	if quoted || content != "TestFoo" {
		t.Fatalf("ident map: content=%q quoted=%v", content, quoted)
	}
	sp, ok := contentSpanToSource(100, srcOf, closeOff, 4, 7) // "Foo"
	if !ok || sp.StartByte != 104 || sp.EndByte != 107 {
		t.Fatalf("ident Foo span %+v ok=%v", sp, ok)
	}

	raw := []byte(`"pre\tpost"`)
	buf = make([]byte, 50+len(raw))
	copy(buf[50:], raw)
	tk = tok{Span: ingestutil.Span{StartByte: 50, EndByte: 50 + uint32(len(raw))}}
	content, srcOf, closeOff, quoted = tokenContentMap(buf, tk)
	if !quoted || content != "pre\tpost" {
		t.Fatalf("quoted content=%q quoted=%v", content, quoted)
	}
	sp, ok = contentSpanToSource(50, srcOf, closeOff, 3, 4)
	if !ok {
		t.Fatal("tab span map failed")
	}
	if sp.StartByte != 50+4 {
		t.Fatalf("tab start=%d want %d", sp.StartByte, 50+4)
	}
	if sp.EndByte != 50+6 {
		t.Fatalf("tab end=%d want %d", sp.EndByte, 50+6)
	}
	sp, ok = contentSpanToSource(50, srcOf, closeOff, 0, len(content))
	if !ok || sp.StartByte != 51 || sp.EndByte != 50+uint32(closeOff) {
		t.Fatalf("full interior %+v closeOff=%d", sp, closeOff)
	}
}

func TestContentSpanToSource_EmptyAndOOB(t *testing.T) {
	src := []byte("abc")
	tk := tok{Span: ingestutil.Span{StartByte: 0, EndByte: 3}}
	_, srcOf, closeOff, _ := tokenContentMap(src, tk)
	sp, ok := contentSpanToSource(0, srcOf, closeOff, 1, 1)
	if !ok || sp.StartByte != 1 || sp.EndByte != 1 {
		t.Fatalf("empty mid %+v ok=%v", sp, ok)
	}
	sp, ok = contentSpanToSource(0, srcOf, closeOff, 3, 3)
	if !ok || sp.StartByte != 3 || sp.EndByte != 3 {
		t.Fatalf("empty end %+v ok=%v", sp, ok)
	}
	if _, ok = contentSpanToSource(0, srcOf, closeOff, -1, 1); ok {
		t.Fatal("want OOB fail")
	}
	if _, ok = contentSpanToSource(0, srcOf, closeOff, 0, 99); ok {
		t.Fatal("want OOB fail")
	}
}

func TestNamedRegexSpan_Ident(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc TestFoo() {}\n")
	path := lewpath.New(dir, "x.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	pat, err := ParsePattern(`(seq (token "func") (capture name (regex "^Test(?P<rest>.*)")))`)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	if len(ms) != 1 {
		t.Fatalf("matches=%d", len(ms))
	}
	names := ms[0].Captures["name"]
	if len(names) == 0 {
		t.Fatal("empty name")
	}
	name := names[0]
	rests := ms[0].Captures["rest"]
	if len(rests) == 0 {
		t.Fatal("empty rest")
	}
	rest := rests[0]
	if name.Text(src) != "TestFoo" {
		t.Fatalf("name=%q", name.Text(src))
	}
	if string(src[name.StartByte:name.EndByte]) != "TestFoo" {
		t.Fatalf("name span %q", src[name.StartByte:name.EndByte])
	}
	if rest.Text(src) != "Foo" {
		t.Fatalf("rest=%q", rest.Text(src))
	}
	if string(src[rest.StartByte:rest.EndByte]) != "Foo" {
		t.Fatalf("rest span %q [%d:%d]", src[rest.StartByte:rest.EndByte], rest.StartByte, rest.EndByte)
	}
	if rest.StartByte < name.StartByte || rest.EndByte > name.EndByte {
		t.Fatalf("rest not inside name: name=[%d:%d) rest=[%d:%d)",
			name.StartByte, name.EndByte, rest.StartByte, rest.EndByte)
	}
}

func TestNamedRegexSpan_StringEscapes(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nvar s = \"pre\\tPOST\"\n")
	path := lewpath.New(dir, "x.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	pat, err := ParsePattern(`(capture s (regex "(?P<head>pre)(?P<tail>.*)"))`)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	if len(ms) != 1 {
		t.Fatalf("matches=%d caps=%v", len(ms), PublicCaptures(ms[0], src))
	}
	sCaps := ms[0].Captures["s"]
	if len(sCaps) == 0 {
		t.Fatal("empty s")
	}
	sCap := sCaps[0]
	heads := ms[0].Captures["head"]
	if len(heads) == 0 {
		t.Fatal("empty head")
	}
	head := heads[0]
	tails := ms[0].Captures["tail"]
	if len(tails) == 0 {
		t.Fatal("empty tail")
	}
	tail := tails[0]
	// Outer capture s is the full token (no CaptureGroup rebind with named groups).
	if sCap.Text(src) != `"pre\tPOST"` {
		t.Fatalf("s=%q", sCap.Text(src))
	}
	if head.Text(src) != "pre" {
		t.Fatalf("head=%q", head.Text(src))
	}
	// Text is always source bytes — escape stays as \t in the file.
	if got := tail.Text(src); got != `\tPOST` {
		t.Fatalf("tail text=%q want \\tPOST", got)
	}
	if got := string(src[tail.StartByte:tail.EndByte]); got != `\tPOST` {
		t.Fatalf("tail span %q", got)
	}
}

func TestNamedRegexSpan_RawString(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nvar s = `hello_world`\n")
	path := lewpath.New(dir, "x.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	pat, err := ParsePattern(`(capture s (regex "hello_(?P<rest>.*)"))`)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	if len(ms) != 1 {
		t.Fatalf("matches=%d", len(ms))
	}
	rests := ms[0].Captures["rest"]
	if len(rests) == 0 {
		t.Fatal("empty rest")
	}
	rest := rests[0]
	if rest.Text(src) != "world" {
		t.Fatalf("rest=%q", rest.Text(src))
	}
}

func TestCaptureGroup_BindsGroupSourceSpan(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nimport \"fmt\"\n\nfunc f(err error) error {\n\treturn fmt.Errorf(\"failed to open: %w\", err)\n}\n")
	path := lewpath.New(dir, "x.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	pat, err := ParsePattern(`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG (regex "(?i)^failed to\\s+(.*)" 1)) "," (capture ERR any) ")")`)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	if len(ms) != 1 {
		t.Fatalf("matches=%d", len(ms))
	}
	msgs := ms[0].Captures["MSG"]
	if len(msgs) == 0 {
		t.Fatal("empty MSG")
	}
	msg := msgs[0]
	// CaptureGroup 1 → outer name is the group span (no quotes).
	if got := msg.Text(src); got != "open: %w" {
		t.Fatalf("MSG.Text=%q want open: %%w", got)
	}
	if string(src[msg.StartByte:msg.EndByte]) != "open: %w" {
		t.Fatalf("MSG span %q", src[msg.StartByte:msg.EndByte])
	}
	got, err := InstantiateEmit([]any{"seq", `"`, []any{"slot"}, `"`}, src, msg, ms[0])
	if err != nil {
		t.Fatal(err)
	}
	if got != `"open: %w"` {
		t.Fatalf("quoted emit=%q", got)
	}
}

func TestRefSelectorSpan(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nimport \"context\"\n\nfunc f() { _ = context.Background() }\n")
	path := lewpath.New(dir, "x.go").String()
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	pat, err := ParsePattern(`(capture c (ref "go:context::Background"))`)
	if err != nil {
		t.Fatal(err)
	}
	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	if len(ms) != 1 {
		t.Fatalf("matches=%d", len(ms))
	}
	cs := ms[0].Captures["c"]
	if len(cs) == 0 {
		t.Fatal("empty c")
	}
	c := cs[0]
	if c.Text(src) != "context.Background" {
		t.Fatalf("c=%q", c.Text(src))
	}
}

func mustMatchFile(t *testing.T, dir, abs, rel string, src []byte, pat Node) []Match {
	t.Helper()
	pf, err := ingestutil.ParseSourceFile(t.Context(), ccgo.Engine{}, abs, "go")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	if err != nil {
		t.Fatal(err)
	}
	hop := ingest.SourceHop(dir, abs)
	hop.Session = testSess()
	hop.Policy = vm
	result, err := ingest.MaterializeSource(t.Context(), hop, ingest.MaterializeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core, err := NodeToPat(pat)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := matchFilePatPol(testSess(), dir, rel, src, pf.Root, core, result, vm.TapePolicy(rel))
	if err != nil {
		t.Fatal(err)
	}
	return ms
}
