// Package tape is the shared structural token tape for a parsed file.
//
// L0 cells are ordered significant tree-sitter leaves. Each cell holds:
//   - Span (byte range only — text is NOT stored; use Span.Text(source))
//   - Type (tree-sitter node type)
//   - Target (optional resolved product ref attached by the caller)
//
// Pattern matching, annotate/highlight, similarity, and future local renames
// should project this tape rather than re-walking the tree.
//
// The DFS walk lives here (language-agnostic). Language-specific leaf rules
// (atomic node types, composite spans) are a Policy — callers do not
// reimplement the walk.
//
// This package does not import pkg/ingest (avoids import cycles). Callers map
// Cell spans to ingestutil.Span at the boundary. Session.TapePolicy supplies
// the leaf Policy; TapeUseTargets is the shared use→target map.
package tape
