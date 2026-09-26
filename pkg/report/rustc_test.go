package report

import (
	"bytes"
	"github.com/lewtec/patlint/pkg/project"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/ingestutil"
)

func TestNormalizeFormatRustc(t *testing.T) {
	t.Parallel()
	got, err := NormalizeFormat("RUSTc")
	if err != nil {
		t.Fatal(err)
	}
	if got != "rustc" {
		t.Fatalf("got %q", got)
	}
	if _, err := NormalizeFormat("html"); err == nil {
		t.Fatal("want error for unknown format")
	}
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
	if err := newRustcWriter(&buf, "", false).write([]Finding{f}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	want := "" +
		"error[demo/hello]: don't greet\n" +
		" --> main.go:3:6\n" +
		"2 | \n" +
		"3 | func \x1b[4mHello\x1b[24m() {}\n"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
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
	if err := newRustcWriter(&buf, "", false).write([]Finding{f}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "1 | \x1b[4mimport unused \"fmt\"\x1b[24m") {
		t.Fatalf("missing underlined import line: %q", got)
	}
	if !strings.Contains(got, "2 |") {
		t.Fatalf("missing neighbor line: %q", got)
	}
	if strings.Contains(got, "2 | \x1b[4m") {
		t.Fatalf("neighbor must not be underlined: %q", got)
	}
	if strings.Contains(got, "3 |") {
		t.Fatalf("only one neighbor: %q", got)
	}
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
	if err := newRustcWriter(&buf, "", false).write([]Finding{f}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "warning[x]: here") {
		t.Fatalf("header: %q", got)
	}
	if !strings.Contains(got, "12 | \x1b[4mfoo\x1b[24m") {
		t.Fatalf("missing underlined snippet: %q", got)
	}
	if strings.Contains(got, "^") {
		t.Fatalf("carets should be gone: %q", got)
	}
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
	if err := newRustcWriter(&buf, "", false).write([]Finding{f}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "help: apply with --fix") {
		t.Fatalf("missing help: %q", got)
	}
	if !strings.Contains(got, "3 - func Hello() {}") {
		t.Fatalf("missing old line: %q", got)
	}
	if !strings.Contains(got, "3 + func Hi() {}") {
		t.Fatalf("missing new line: %q", got)
	}
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
	if err := newRustcWriter(&buf, "", false).write([]Finding{f}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "note: fix skipped: overlap") {
		t.Fatalf("missing skip note: %q", got)
	}
	if strings.Contains(got, "apply with --fix") {
		t.Fatalf("skipped should not suggest --fix: %q", got)
	}
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
	if err := newRustcWriter(&buf, "/tmp", true).write([]Finding{f}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected ANSI, got %q", got)
	}
	if !strings.Contains(got, "\x1b]8;;file://") {
		t.Fatalf("expected OSC 8 file link, got %q", got)
	}
	if !strings.Contains(got, "package") || !strings.Contains(got, "main") {
		t.Fatalf("missing source text: %q", got)
	}
	if !strings.Contains(got, "\x1b[4m") {
		t.Fatalf("expected underline SGR, got %q", got)
	}
	if strings.Contains(got, "^^^^^") {
		t.Fatalf("carets should be gone: %q", got)
	}
}

func TestUnderlineSpanSkipsANSI(t *testing.T) {
	t.Parallel()
	// Reset after the first styled rune used to kill underline.
	styled := "(\x1b[35mlanguageName\x1b[0m, filename"
	got := underlineSpan(styled, 1, 26)
	if strings.Count(got, "\x1b[4m") < 2 {
		t.Fatalf("underline not reasserted after reset: %q", got)
	}
	if !strings.Contains(got, "languageName") || !strings.Contains(got, "filename") {
		t.Fatalf("missing span text: %q", got)
	}
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
	if err := WriteFormat(&buf, "rustc", "", []Finding{f}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "error[x]: m") {
		t.Fatalf("got %q", buf.String())
	}
}
