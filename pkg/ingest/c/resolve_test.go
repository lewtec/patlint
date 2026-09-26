package c

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingest"
)

func TestResolveIncludeQuotedRelative(t *testing.T) {
	ctx := ingest.ImportResolveContext{
		RootDir:      t.TempDir(),
		ImporterPath: "src/main.c",
		KnownFiles:   map[string]bool{"src/helper.h": true},
	}
	got := ResolveInclude("helper.h", ctx)
	want := "path:./src/helper.h"
	if got != want {
		t.Fatalf("ResolveInclude(helper.h) = %q, want %q", got, want)
	}
}

func TestResolveIncludeSystem(t *testing.T) {
	got := ResolveInclude("<stdio.h>", ingest.ImportResolveContext{})
	if got != "c:stdio.h" {
		t.Fatalf("system include = %q, want c:stdio.h", got)
	}
	got = ResolveInclude("sys:stdint.h", ingest.ImportResolveContext{})
	if got != "c:stdint.h" {
		t.Fatalf("sys: tag = %q, want c:stdint.h", got)
	}
}

func TestResolveIncludeDotDot(t *testing.T) {
	ctx := ingest.ImportResolveContext{
		ImporterPath: "pkg/a/x.c",
		KnownFiles:   map[string]bool{"pkg/b/y.h": true},
	}
	got := ResolveInclude("../b/y.h", ctx)
	want := "path:./pkg/b/y.h"
	if got != want {
		t.Fatalf("ResolveInclude(../b/y.h) = %q, want %q", got, want)
	}
}
