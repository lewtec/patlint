package highlight_test

import (
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"strings"
	"testing"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/ui/highlight"
)

func TestLinesPlainSplitsSource(t *testing.T) {
	src := []byte("package main\n\nfunc Hello() {}\n")
	got := highlight.Lines(src, "main.go", highlight.Options{Color: false})
	want := []string{"package main", "", "func Hello() {}", ""}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestLinesColorStylesKeywords(t *testing.T) {
	src := []byte("package main\n")
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	got := highlight.Lines(src, "main.go", highlight.Options{Color: true, Walker: w})
	if len(got) < 1 {
		t.Fatal("expected at least one line")
	}
	if !strings.Contains(got[0], "\x1b[") {
		t.Fatalf("expected ANSI on line 0, got %q", got[0])
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "package") || !strings.Contains(joined, "main") {
		t.Fatalf("colored lines missing source text: %q", joined)
	}
}

func TestLinesEmpty(t *testing.T) {
	if got := highlight.Lines(nil, "main.go", highlight.Options{Color: true}); got != nil {
		t.Fatalf("got %#v", got)
	}
}
