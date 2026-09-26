package ingestutil

import "testing"

func TestEmitSeq(t *testing.T) {
	got := EmitSeq([]string{"package", "pkg"}, map[string]string{"pkg": "main"})
	if got != "package main" {
		t.Fatalf("package=%q", got)
	}
	got = EmitSeq([]string{"import", `"`, "path", `"`}, map[string]string{"path": "fmt"})
	if got != `import "fmt"` {
		t.Fatalf("import=%q", got)
	}
	got = EmitSeq([]string{"from", "path", "import", "leaf"}, map[string]string{"path": "os.path", "leaf": "join"})
	if got != "from os.path import join" {
		t.Fatalf("from=%q", got)
	}
	got = EmitSeq([]string{"from", "path", "import", "leaf"}, map[string]string{"path": ".utils", "leaf": "helper"})
	if got != "from .utils import helper" {
		t.Fatalf("rel=%q", got)
	}
	got = EmitSeq([]string{"import", "{", "leaf", "}", "from", "'", "path", "'", ";"}, map[string]string{"path": "./x", "leaf": "A"})
	if got != "import { A } from './x';" {
		t.Fatalf("esm=%q", got)
	}
	got = EmitSeq([]string{"const", "qual", "=", "@import", "(", `"`, "path", `"`, ")", ";"}, map[string]string{"path": "./a", "qual": "foo"})
	if got != `const foo = @import("./a");` {
		t.Fatalf("zig=%q", got)
	}
	got = EmitSeq([]string{"prefix-", "X", "-suffix"}, nil)
	if got != "prefix-X-suffix" {
		t.Fatalf("slot wrap=%q", got)
	}
	got = EmitSeq([]string{"fmt.Errorf", `("open image: %w", err)`}, nil)
	if got != `fmt.Errorf("open image: %w", err)` {
		t.Fatalf("ref+call=%q", got)
	}
	got = EmitSeq([]string{"from", ".utils", "import", "helper"}, nil)
	if got != "from .utils import helper" {
		t.Fatalf("rel-dot=%q", got)
	}
}
