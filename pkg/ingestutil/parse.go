package ingestutil

import (
	"fmt"
	"os"

	"github.com/lewtec/patlint/pkg/sitter"
)

var (
	ErrUnsupportedLanguage = sitter.ErrUnsupportedLanguage
	ErrSetLanguage         = sitter.ErrSetLanguage
)

// ParsedFile is a one-shot parse of a source file.
// Close is safe on nil or double-close; the CST is a GC snapshot.
type ParsedFile struct {
	Source []byte
	Root   *sitter.Node
}

// Close drops the root. Safe on nil or double-close.
func (p *ParsedFile) Close() {
	if p == nil {
		return
	}
	p.Root = nil
}

// RequireGrammar reports whether eng has id.
func RequireGrammar(eng sitter.Engine, id string) error {
	if eng == nil {
		return sitter.ErrNilEngine
	}
	if id == "" {
		return fmt.Errorf("%w: empty language", ErrUnsupportedLanguage)
	}
	if !eng.Has(id) {
		return fmt.Errorf("%w: %q (grammar not registered)", ErrUnsupportedLanguage, id)
	}
	return nil
}

// ParseSource parses content as language id (pack as-language / AttributeHost).
// path is for errors and labels only — not extension lookup.
func ParseSource(eng sitter.Engine, content []byte, path, language string) (*ParsedFile, error) {
	if eng == nil {
		return nil, sitter.ErrNilEngine
	}
	if language == "" {
		return nil, fmt.Errorf("%w for %s (empty language; use pack path attribution)", ErrUnsupportedLanguage, path)
	}
	tree, err := eng.Parse(content, language)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var root *sitter.Node
	if tree != nil {
		root = tree.Root
	}
	return &ParsedFile{Source: content, Root: root}, nil
}

// ParseSourceFile reads path and parses it as language id (not by extension).
func ParseSourceFile(eng sitter.Engine, path, language string) (*ParsedFile, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSource(eng, source, path, language)
}
