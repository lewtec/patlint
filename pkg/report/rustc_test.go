package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"

	"github.com/google/go-cmp/cmp"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/ingestutil"
)

func TestNormalizeFormatRustc(t *testing.T) {
	t.Parallel()
	got, err := NormalizeFormat("RUSTc")
	require.NoError(t, err)
	require.Equal(t, "rustc", got)
	_, err = NormalizeFormat("html")
	require.Error(t, err, "want error for unknown format")

}

func TestWriteRustcSnippetUnderline(t *testing.T) {
	t.Parallel()
	src := []byte("package main\n\nfunc Hello() {}\n")
	// "func Hello() {}" is line 3; Hello is columns 6-11.
	f := Finding{
		RuleID:  "demo/hello",
		Level:   LevelError,
		Message: "don't greet",
		File:    "main.go",
		Line:    3,
		Column:  6,
		EndLine: 3,
		EndCol:  11,
		Snippet: "Hello",
		Source:  src,
	}
	var buf bytes.Buffer
	err := newRustcWriter(t.Context(), &buf, "", false).write([]Finding{f})
	require.NoError(t, err)

	got := buf.String()
	want := "" +
		"error[demo/hello]: don't greet\n" +
		" --> main.go:3:6\n" +
		"2 | \n" +
		"3 | func \x1b[4mHello\x1b[24m() {}\n"
	diff := cmp.Diff(want, got)
	require.Empty(t, diff, "mismatch (-want +got):\n%s", diff)

}

func TestWriteRustcExclusiveNewlineEnd(t *testing.T) {
	t.Parallel()
	src := []byte("import unused \"fmt\"\n\nfunc main() {}\n")
	f := Finding{
		RuleID:  "imports/unused-named",
		Level:   LevelWarning,
		Message: "Unused named import",
		File:    "main.go",
		Line:    1,
		Column:  1,
		EndLine: 2,
		EndCol:  1,
		Source:  src,
	}
	var buf bytes.Buffer
	err := newRustcWriter(t.Context(), &buf, "", false).write([]Finding{f})
	require.NoError(t, err)

	got := buf.String()
	require.Contains(t, got, "1 | \x1b[4mimport unused \"fmt\"\x1b[24m")
	require.Contains(t, got, "2 |")
	require.NotContains(t, got, "2 | \x1b[4m")
	require.NotContains(t, got, "3 |")

}

func TestWriteRustcSnippetFallback(t *testing.T) {
	t.Parallel()
	f := Finding{
		RuleID:  "x",
		Level:   LevelWarning,
		Message: "here",
		File:    "a.go",
		Line:    12,
		Column:  1,
		EndLine: 12,
		EndCol:  4,
		Snippet: "foo",
	}
	var buf bytes.Buffer
	err := newRustcWriter(t.Context(), &buf, "", false).write([]Finding{f})
	require.NoError(t, err)

	got := buf.String()
	require.Contains(t, got, "warning[x]: here")
	require.Contains(t, got, "12 | \x1b[4mfoo\x1b[24m")
	require.NotContains(t, got, "^")

}

func TestWriteRustcFixDiff(t *testing.T) {
	t.Parallel()
	src := []byte("package main\n\nfunc Hello() {}\n")
	// Hello at bytes: "package main\n\nfunc " = 13+1+5 = 19, "Hello" = 19..24
	f := Finding{
		RuleID:  "demo/hello",
		Level:   LevelError,
		Message: "don't greet",
		File:    "main.go",
		Line:    3,
		Column:  6,
		EndLine: 3,
		EndCol:  11,
		Source:  src,
		Fixable: true,
		SiteEdits: []project.Edit{{
			File:    "main.go",
			Span:    ingestutil.Span{StartByte: 19, EndByte: 24},
			NewText: "Hi",
		}},
	}
	var buf bytes.Buffer
	err := newRustcWriter(t.Context(), &buf, "", false).write([]Finding{f})
	require.NoError(t, err)

	got := buf.String()
	require.Contains(t, got, "help: apply with --fix")
	require.Contains(t, got, "3 - func Hello() {}")
	require.Contains(t, got, "3 + func Hi() {}")

}

func TestWriteRustcFixSkipped(t *testing.T) {
	t.Parallel()
	f := Finding{
		RuleID:     "x",
		Level:      LevelNote,
		Message:    "m",
		File:       "a.go",
		Line:       1,
		Column:     1,
		EndLine:    1,
		EndCol:     2,
		Snippet:    "a",
		Fixable:    true,
		FixSkipped: true,
	}
	var buf bytes.Buffer
	err := newRustcWriter(t.Context(), &buf, "", false).write([]Finding{f})
	require.NoError(t, err)

	got := buf.String()
	require.Contains(t, got, "note: fix skipped: overlap")
	require.NotContains(t, got, "apply with --fix")

}

func TestWriteRustcColorAndLink(t *testing.T) {
	src := []byte("package main\n")
	f := Finding{
		RuleID:  "x",
		Level:   LevelError,
		Message: "m",
		File:    "main.go",
		Line:    1,
		Column:  1,
		EndLine: 1,
		EndCol:  8,
		Source:  src,
	}
	var buf bytes.Buffer
	err := newRustcWriter(t.Context(), &buf, "/tmp", true).write([]Finding{f})
	require.NoError(t, err)

	got := buf.String()
	require.Contains(t, got, "\x1b[")
	require.Contains(t, got, "\x1b]8;;file://")
	require.Contains(t, got, "package")
	require.Contains(t, got, "main")
	require.Contains(t, got, "\x1b[4m")
	require.NotContains(t, got, "^^^^^")

}

func TestUnderlineSpanSkipsANSI(t *testing.T) {
	t.Parallel()
	// Reset after the first styled rune used to kill underline.
	styled := "(\x1b[35mlanguageName\x1b[0m, filename"
	got := underlineSpan(styled, 1, 26)
	require.GreaterOrEqual(t, strings.Count(got, "\x1b[4m"), 2, "underline not reasserted after reset: %q", got)
	require.Contains(t, got, "languageName")
	require.Contains(t, got, "filename")

}

func TestWriteFormatRustc(t *testing.T) {
	t.Parallel()
	f := Finding{
		RuleID:  "x",
		Level:   LevelError,
		Message: "m",
		File:    "a.go",
		Line:    1,
		Column:  1,
		Snippet: "z",
		EndCol:  2,
		EndLine: 1,
	}
	var buf bytes.Buffer
	err := WriteFormat(t.Context(), &buf, "rustc", "", []Finding{f}, nil)
	require.NoError(t, err)
	require.Contains(t, buf.String(), "error[x]: m")

}
