// Package prelude is the load entry for patlint.
//
//	import _ "github.com/lewtec/patlint/internal/prelude"
//
// It blank-imports the Go import resolver and embeds the .rft packs.
// Grammars register in pkg/sitter/treesitter and are parsed through the lewkit
// tree-sitter driver.
package prelude

import (
	"embed"

	// Go import resolution for @ref. Python and JavaScript rules match tokens
	// from the embedded language packs; they do not need a Go driver.
	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

//go:embed *.rft
var FS embed.FS
