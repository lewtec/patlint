package pattern

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestUnquoteLiteralMap_Escapes(t *testing.T) {
	raw := []byte(`"a\tb\n\\"`)
	content, srcOf, closeOff, ok := unquoteLiteralMap(raw)
	require.True(t, ok,
		"unquote failed")

	wantContent := "a\tb\n\\"
	require.Equal(t, wantContent, content,
		"content=%q want %q", content, wantContent)
	require.Equal(t, len(raw)-1, closeOff,
		"closeOff=%d want %d", closeOff, len(raw)-1)
	require.Len(t, srcOf, len(content),
		"srcOf len=%d content len=%d", len(srcOf), len(content))
	require.Equal(t, 1, srcOf[0],
		"srcOf[0]=%d want 1", srcOf[0])
	require.Equal(t, 2, srcOf[1],
		"srcOf[1]=%d want 2 (start of \\t)", srcOf[1])
	require.Equal(t, 4, srcOf[2],
		"srcOf[2]=%d want 4", srcOf[2])

}

func TestContentSpanToSource_IdentAndQuoted(t *testing.T) {
	ident := []byte("TestFoo")
	buf := make([]byte, 100+len(ident))
	copy(buf[100:], ident)
	tk := tok{Span: ingestutil.Span{StartByte: 100, EndByte: 100 + uint32(len(ident))}}
	content, srcOf, closeOff, quoted := tokenContentMap(buf, tk)
	require.False(t, quoted || content != "TestFoo",
		"ident map: content=%q quoted=%v", content, quoted)

	sp, ok := contentSpanToSource(100, srcOf, closeOff, 4, 7)
	// "Foo"
	require.True(t, ok, "ident Foo span %+v", sp)
	require.Equal(t, uint32(104), sp.StartByte)
	require.Equal(t, uint32(107), sp.EndByte)

	raw := []byte(`"pre\tpost"`)
	buf = make([]byte, 50+len(raw))
	copy(buf[50:], raw)
	tk = tok{Span: ingestutil.Span{StartByte: 50, EndByte: 50 + uint32(len(raw))}}
	content, srcOf, closeOff, quoted = tokenContentMap(buf, tk)
	require.True(t, quoted)
	require.Equal(t, "pre\tpost", content)

	sp, ok = contentSpanToSource(50, srcOf, closeOff, 3, 4)
	require.True(t, ok, "tab span map failed")
	require.Equal(t, uint32(50+4), sp.StartByte)
	require.Equal(t, uint32(50+6), sp.EndByte)

	sp, ok = contentSpanToSource(50, srcOf, closeOff, 0, len(content))
	require.True(t, ok, "full interior %+v closeOff=%d", sp, closeOff)
	require.Equal(t, uint32(51), sp.StartByte)
	require.Equal(t, uint32(50+closeOff), sp.EndByte)

}

func TestContentSpanToSource_EmptyAndOOB(t *testing.T) {
	src := []byte("abc")
	tk := tok{Span: ingestutil.Span{StartByte: 0, EndByte: 3}}
	_, srcOf, closeOff, _ := tokenContentMap(src, tk)
	sp, ok := contentSpanToSource(0, srcOf, closeOff, 1, 1)
	require.True(t, ok, "empty mid %+v", sp)
	require.Equal(t, uint32(1), sp.StartByte)
	require.Equal(t, uint32(1), sp.EndByte)

	sp, ok = contentSpanToSource(0, srcOf, closeOff, 3, 3)
	require.True(t, ok, "empty end %+v", sp)
	require.Equal(t, uint32(3), sp.StartByte)
	require.Equal(t, uint32(3), sp.EndByte)

	_, ok = contentSpanToSource(0, srcOf, closeOff, -1, 1)
	require.False(t, ok,
		"want OOB fail")

	_, ok = contentSpanToSource(0, srcOf, closeOff, 0, 99)
	require.False(t, ok,
		"want OOB fail")

}

func TestNamedRegexSpan_Ident(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nfunc TestFoo() {}\n")
	path := lewpath.New(dir, "x.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(seq (token "func") (capture name (regex "^Test(?P<rest>.*)")))`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d", len(ms))

	names := ms[0].Captures["name"]
	require.NotEmpty(t, names,
		"empty name")

	name := names[0]
	rests := ms[0].Captures["rest"]
	require.NotEmpty(t, rests,
		"empty rest")

	rest := rests[0]
	require.Equal(t, "TestFoo", name.Text(src),
		"name=%q", name.Text(src))
	require.Equal(t, "TestFoo", string(src[name.StartByte:name.EndByte]),
		"name span %q", src[name.StartByte:name.EndByte])
	require.Equal(t, "Foo", rest.Text(src),
		"rest=%q", rest.Text(src))
	require.Equal(t, "Foo", string(src[rest.StartByte:rest.EndByte]),
		"rest span %q [%d:%d]", src[rest.StartByte:rest.EndByte], rest.StartByte, rest.EndByte)
	require.False(t, rest.StartByte < name.StartByte || rest.EndByte > name.EndByte,
		"rest not inside name: name=[%d:%d) rest=[%d:%d)",
		name.StartByte, name.EndByte, rest.StartByte, rest.EndByte)

}

func TestNamedRegexSpan_StringEscapes(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nvar s = \"pre\\tPOST\"\n")
	path := lewpath.New(dir, "x.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(capture s (regex "(?P<head>pre)(?P<tail>.*)"))`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d caps=%v", len(ms), PublicCaptures(ms[0], src))

	sCaps := ms[0].Captures["s"]
	require.NotEmpty(t, sCaps,
		"empty s")

	sCap := sCaps[0]
	heads := ms[0].Captures["head"]
	require.NotEmpty(t, heads,
		"empty head")

	head := heads[0]
	tails := ms[0].Captures["tail"]
	require.NotEmpty(t, tails,
		"empty tail")

	tail := tails[0]
	// Outer capture s is the full token (no CaptureGroup rebind with named groups).
	require.Equal(t, `"pre\tPOST"`, sCap.Text(src))
	require.Equal(t, "pre", head.Text(src),
		"head=%q", head.Text(src))
	{

		// Text is always source bytes — escape stays as \t in the file.
		got := tail.Text(src)
		require.Equal(t, `\tPOST`, got,
			"tail text=%q want \\tPOST", got)
	}

	got := string(src[tail.StartByte:tail.EndByte])
	require.Equal(t, `\tPOST`, got,
		"tail span %q", got)

}

func TestNamedRegexSpan_RawString(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nvar s = `hello_world`\n")
	path := lewpath.New(dir, "x.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(capture s (regex "hello_(?P<rest>.*)"))`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d", len(ms))

	rests := ms[0].Captures["rest"]
	require.NotEmpty(t, rests,
		"empty rest")

	rest := rests[0]
	require.Equal(t, "world", rest.Text(src),
		"rest=%q", rest.Text(src))

}

func TestCaptureGroup_BindsGroupSourceSpan(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nimport \"fmt\"\n\nfunc f(err error) error {\n\treturn fmt.Errorf(\"failed to open: %w\", err)\n}\n")
	path := lewpath.New(dir, "x.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG (regex "(?i)^failed to\\s+(.*)" 1)) "," (capture ERR any) ")")`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d", len(ms))

	msgs := ms[0].Captures["MSG"]
	require.NotEmpty(t, msgs,
		"empty MSG")

	msg := msgs[0]
	{
		// CaptureGroup 1 → outer name is the group span (no quotes).
		got := msg.Text(src)
		require.Equal(t, "open: %w", got,
			"MSG.Text=%q want open: %%w", got)
	}
	require.Equal(t, "open: %w", string(src[msg.StartByte:msg.EndByte]),
		"MSG span %q", src[msg.StartByte:msg.EndByte])

	got, err := InstantiateEmit([]any{"seq", `"`, []any{"slot"}, `"`}, src, msg, ms[0])
	require.NoError(t, err)
	require.Equal(t, `"open: %w"`, got,
		"quoted emit=%q", got)

}

func TestRefSelectorSpan(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\n\nimport \"context\"\n\nfunc f() { _ = context.Background() }\n")
	path := lewpath.New(dir, "x.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(`(capture c (ref "go:context::Background"))`)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d", len(ms))

	cs := ms[0].Captures["c"]
	require.NotEmpty(t, cs,
		"empty c")

	c := cs[0]
	require.Equal(t, "context.Background", c.Text(src),
		"c=%q", c.Text(src))

}

func mustMatchFile(t *testing.T, dir, abs, rel string, src []byte, pat Node) []Match {
	t.Helper()
	pf, err := ingestutil.ParseSourceFile(t.Context(), treesitter.Engine{}, abs, "go")
	require.NoError(t, err)

	defer pf.Close()
	vm, err := New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	hop := ingest.SourceHop(dir, abs)
	hop.Session = testSess()
	hop.Policy = vm
	result, err := ingest.MaterializeSource(t.Context(), hop, ingest.MaterializeOptions{})
	require.NoError(t, err)

	core, err := NodeToPat(pat)
	require.NoError(t, err)

	ms, err := matchFilePatPol(testSess(), dir, rel, src, pf.Root, core, result, vm.TapePolicy(rel))
	require.NoError(t, err)

	return ms
}
