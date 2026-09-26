// Package prelude is the single “load everything” entry for refactree.
//
//	import _ "github.com/lewtec/patlint/internal/prelude"
//
// Responsibilities (all here, not in pkg/pattern):
//   - blank-import every language surface (drivers, families, move)
//   - go:embed all *.rft files as FS (pass to pattern.New)
//
// Grammars register on the ccgo engine (pkg/sitter). Filenames under this
// package are organizational only.
package prelude

import (
	"embed"

	// Language surfaces (drivers, families, move — not grammars).
	_ "github.com/lewtec/patlint/pkg/ingest/c"
	_ "github.com/lewtec/patlint/pkg/ingest/ecma/js"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	_ "github.com/lewtec/patlint/pkg/ingest/go/templ"
	_ "github.com/lewtec/patlint/pkg/ingest/html"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/java"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/kotlin"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/scala"
	_ "github.com/lewtec/patlint/pkg/ingest/nix"
	_ "github.com/lewtec/patlint/pkg/ingest/python"
	_ "github.com/lewtec/patlint/pkg/ingest/rust"
	_ "github.com/lewtec/patlint/pkg/ingest/zig"
)

//go:embed *.rft
var FS embed.FS
