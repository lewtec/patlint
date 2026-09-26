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

	// Go import resolution for @ref. Python and JavaScript rules match tokens
	// from the embedded language packs; they do not need a Go driver.
	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

//go:embed *.rft
var FS embed.FS
