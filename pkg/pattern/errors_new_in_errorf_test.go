package pattern

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"
)

// Pattern under test: nested errors.New inside fmt.Errorf (any arg position).
const errNewInErrorfPat = `(seq (capture F (ref "go:fmt::Errorf")) "(" (* any) (capture E (ref "go:errors::New")) "(" (* any) ")" (* any) ")")`

func TestErrorsNewInErrorf_SingleSiteCaptures(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import (
	"errors"
	"fmt"
)

func f() error {
	return fmt.Errorf("boom: %w", errors.New("inner"))
}
`)
	path := lewpath.New(dir, "x.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example.com/t\n\ngo 1.22\n"), 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(errNewInErrorfPat)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "x.go", src, pat)
	require.Len(t, ms, 1,
		"matches=%d want 1; caps=%v", len(ms), capsOf(ms, src))

	m := ms[0]

	// Match span should be the call, not the whole file.
	gotSpan := m.Span.Text(src)
	wantSpan := `fmt.Errorf("boom: %w", errors.New("inner"))`
	require.Equal(t, wantSpan, gotSpan,
		"match span=%q want %q", gotSpan, wantSpan)

	f := m.Captures["F"]
	require.Len(t, f, 1,
		"F sites=%d want 1; caps=%v", len(f), PublicCaptures(m, src))
	{

		got := f[0].Text(src)
		require.Equal(t, "fmt.Errorf", got,
			"F=%q want fmt.Errorf", got)
	}

	e := m.Captures["E"]
	require.Len(t, e, 1,
		"E sites=%d want 1; caps=%v", len(e), PublicCaptures(m, src))

	got := e[0].Text(src)
	require.Equal(t, "errors.New", got,
		"E=%q want errors.New", got)

}

func TestErrorsNewInErrorf_Variants(t *testing.T) {
	dir := t.TempDir()
	src := []byte(`package p

import (
	"errors"
	"fmt"
	stderrors "errors"
)

func hitOneLine() error {
	return fmt.Errorf("boom: %w", errors.New("inner"))
}

func hitMultiLine() error {
	return fmt.Errorf(
		"boom: %w",
		errors.New("inner"),
	)
}

func hitAlias() error {
	return fmt.Errorf("x: %w", stderrors.New("y"))
}

func hitThreeArgs() error {
	return fmt.Errorf("a %v %w", 1, errors.New("z"))
}

func missWrapExisting() error {
	err := errors.New("pre")
	return fmt.Errorf("wrap: %w", err)
}

func missIndirect() error {
	f := errors.New
	return fmt.Errorf("no: %w", f("x"))
}
`)
	path := lewpath.New(dir, "x.go").String()
	{
		err := os.WriteFile(path, src, 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(dir, "go.mod").String(), []byte("module example.com/t\n\ngo 1.22\n"), 0o644)
		require.NoError(t, err)
	}

	pat, err := ParsePattern(errNewInErrorfPat)
	require.NoError(t, err)

	ms := mustMatchFile(t, dir, path, "x.go", src, pat)

	// Ideal: exactly the four hit* sites. Report what we got.
	type want struct {
		sub string // substring of match span
		f   string
		e   string
	}
	wants := []want{
		{`fmt.Errorf("boom: %w", errors.New("inner"))`, "fmt.Errorf", "errors.New"},
		{"errors.New(\"inner\")", "fmt.Errorf", "errors.New"}, // multi-line contains this
		{`fmt.Errorf("x: %w", stderrors.New("y"))`, "fmt.Errorf", "stderrors.New"},
		{`fmt.Errorf("a %v %w", 1, errors.New("z"))`, "fmt.Errorf", "errors.New"},
	}

	if len(ms) != len(wants) {
		t.Errorf("matches=%d want %d; caps=%v", len(ms), len(wants), capsOf(ms, src))
		for i, m := range ms {
			sp := m.Span.Text(src)
			if len(sp) > 100 {
				sp = sp[:100] + "…"
			}
			t.Logf("  #%d bytes=[%d,%d) %q caps=%v", i, m.Span.StartByte, m.Span.EndByte, sp, PublicCaptures(m, src))
		}
	}

	// Index hits by containing the distinctive call text.
	byKey := map[string]Match{}
	for _, m := range ms {
		sp := m.Span.Text(src)
		switch {
		case strings.Contains(sp, `"boom: %w", errors.New("inner")`) && !strings.Contains(sp, "\n"):
			byKey["oneLine"] = m
		case strings.Contains(sp, "errors.New(\"inner\")") && strings.Contains(sp, "\n"):
			byKey["multiLine"] = m
		case strings.Contains(sp, "stderrors.New"):
			byKey["alias"] = m
		case strings.Contains(sp, `"a %v %w"`):
			byKey["threeArgs"] = m
		default:
			t.Logf("unexpected match span=%q caps=%v", trim(sp, 120), PublicCaptures(m, src))
		}
	}

	check := func(name, wantF, wantE string) {
		t.Helper()
		m, ok := byKey[name]
		if !ok {
			t.Errorf("missing hit %s", name)
			return
		}
		if got := captureText(m, "F", src); got != wantF {
			t.Errorf("%s F=%q want %q", name, got, wantF)
		}
		if got := captureText(m, "E", src); got != wantE {
			t.Errorf("%s E=%q want %q", name, got, wantE)
		}
		// Whole-file / cross-function spans are wrong captures for a call site.
		if m.Span.StartByte == 0 && int(m.Span.EndByte) == len(src) {
			t.Errorf("%s span covers entire file", name)
		}
		if strings.Contains(m.Span.Text(src), "missWrapExisting") || strings.Contains(m.Span.Text(src), "missIndirect") {
			t.Errorf("%s span leaks into miss* funcs: %q", name, trim(m.Span.Text(src), 80))
		}
	}
	check("oneLine", "fmt.Errorf", "errors.New")
	check("multiLine", "fmt.Errorf", "errors.New")
	check("alias", "fmt.Errorf", "stderrors.New")
	check("threeArgs", "fmt.Errorf", "errors.New")

	// Misses must not appear as dedicated match spans.
	for _, m := range ms {
		sp := m.Span.Text(src)
		if sp == `fmt.Errorf("wrap: %w", err)` {
			t.Errorf("matched wrap-existing site: %q", sp)
		}
		if sp == `fmt.Errorf("no: %w", f("x"))` {
			t.Errorf("matched indirect New site: %q", sp)
		}
	}
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
