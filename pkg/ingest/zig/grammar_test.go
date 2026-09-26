package zig_test

import (
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestGrammarRegistered(t *testing.T) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	if got := vm.FamilyForLanguage("zig"); got != "zig" {
		t.Fatalf("FamilyForLanguage(zig)=%q", got)
	}
	if lang, ok := vm.HostLanguage("foo.zig"); !ok || lang != "zig" {
		t.Fatalf("LanguageForFile=.zig -> %q ok=%v", lang, ok)
	}
	src := []byte("pub fn main() void {}\n")
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, src, "main.zig", "zig")
	if err != nil {
		t.Fatal(err)
	}
	if pf == nil || pf.Root == nil {
		t.Fatal("nil parse")
	}
	t.Logf("root=%s", pf.Root.Type())
}

func TestTreeSitterGrammarDriver(t *testing.T) {
	if err := ingestutil.RequireGrammar(ccgo.Engine{}, "zig"); err != nil {
		t.Fatal(err)
	}
}
