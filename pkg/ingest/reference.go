package ingest

import (
	refpkg "github.com/lewtec/patlint/pkg/reference"
)

// Reference is the canonical reference model shared across packages.
type Reference = refpkg.Reference

func ParseReference(s string) Reference {
	return refpkg.Parse(s)
}

// HighlightLanguageProvider is an optional reference provider capability:
// the pack/host language id to use for paint/fences when the ref has no path
// claim (e.g. node:react → javascript). Family leftover, not a concentrator table.
type HighlightLanguageProvider interface {
	HighlightLanguage() string
}

func FileRef(path string) string {
	return refpkg.FileRef(path)
}

func AtomRef(path, symbol string) string {
	return refpkg.AtomRef(path, symbol)
}

// LastPathComponent returns the final slash-separated path segment.
func LastPathComponent(s string) string {
	return refpkg.LastPathComponent(s)
}
