package highlight_test

import (
	"bytes"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"strings"
	"testing"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/ui/highlight"
)

func TestWritePlainCopiesSource(t *testing.T) {
	src := []byte("package main\n\nfunc Hello() {}\n")
	var buf bytes.Buffer
	if err := highlight.Write(&buf, src, "main.go", highlight.Options{Color: false}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), src) {
		t.Fatalf("plain write changed source:\n got %q\nwant %q", buf.Bytes(), src)
	}
}

func TestWriteColorStylesKeywords(t *testing.T) {
	src := []byte("package main\n")
	var buf bytes.Buffer
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := highlight.Write(&buf, src, "main.go", highlight.Options{Color: true, Walker: w}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// lipgloss / termenv emit ESC sequences when Color is forced.
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI escapes in colored output, got %q", out)
	}
	// Visible text still contains the source words.
	if !strings.Contains(out, "package") || !strings.Contains(out, "main") {
		t.Fatalf("colored output missing source text: %q", out)
	}
}

func TestWriteUnsupportedLanguageFallsBack(t *testing.T) {
	src := []byte("just plain text\n")
	var buf bytes.Buffer
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := highlight.Write(&buf, src, "notes.txt", highlight.Options{Color: true, Walker: w}); err != nil {
		t.Fatal(err)
	}
	// Unparseable: still returns the full source (with or without spans).
	if !strings.Contains(buf.String(), "just plain text") {
		t.Fatalf("missing source: %q", buf.String())
	}
}

func TestAutoColorRespectsNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if highlight.AutoColor(nil) {
		t.Fatal("NO_COLOR set: AutoColor must be false")
	}
}
