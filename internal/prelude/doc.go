// Package prelude loads the product world once.
//
//	import _ "github.com/lewtec/patlint/internal/prelude"
//
// - blank-imports language surfaces (drivers, families, move)
// - embeds all *.rft here as FS; pass to pattern.New
//
// Grammars register on the ccgo engine (pkg/sitter). Do not blank-import
// grammars from pkg/ingest language packages.
//
// pkg/pattern is the sexpr/match/extract engine only — it does not embed or
// auto-load prelude files.
package prelude
