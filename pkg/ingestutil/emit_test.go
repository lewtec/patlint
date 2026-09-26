package ingestutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmitSeq(t *testing.T) {
	got := EmitSeq([]string{"package", "pkg"}, map[string]string{"pkg": "main"})
	require.Equal(t, "package main", got,
		"package=%q", got)

	got = EmitSeq([]string{"import", `"`, "path", `"`}, map[string]string{"path": "fmt"})
	require.Equal(t, `import "fmt"`, got,
		"import=%q", got)

	got = EmitSeq([]string{"from", "path", "import", "leaf"}, map[string]string{"path": "os.path", "leaf": "join"})
	require.Equal(t, "from os.path import join", got,
		"from=%q", got)

	got = EmitSeq([]string{"from", "path", "import", "leaf"}, map[string]string{"path": ".utils", "leaf": "helper"})
	require.Equal(t, "from .utils import helper", got,
		"rel=%q", got)

	got = EmitSeq([]string{"import", "{", "leaf", "}", "from", "'", "path", "'", ";"}, map[string]string{"path": "./x", "leaf": "A"})
	require.Equal(t, "import { A } from './x';", got,
		"esm=%q", got)

	got = EmitSeq([]string{"const", "qual", "=", "@import", "(", `"`, "path", `"`, ")", ";"}, map[string]string{"path": "./a", "qual": "foo"})
	require.Equal(t, `const foo = @import("./a");`, got,
		"zig=%q", got)

	got = EmitSeq([]string{"prefix-", "X", "-suffix"}, nil)
	require.Equal(t, "prefix-X-suffix", got,
		"slot wrap=%q", got)

	got = EmitSeq([]string{"fmt.Errorf", `("open image: %w", err)`}, nil)
	require.Equal(t, `fmt.Errorf("open image: %w", err)`, got,
		"ref+call=%q", got)

	got = EmitSeq([]string{"from", ".utils", "import", "helper"}, nil)
	require.Equal(t, "from .utils import helper", got,
		"rel-dot=%q", got)

}
