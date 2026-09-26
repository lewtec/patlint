package sitter

import (
	"context"
	"errors"
	"fmt"
)

var (
	// ErrNilEngine is returned when parse is asked with no Engine on the Session.
	ErrNilEngine = errors.New("nil engine")
	// ErrUnsupportedLanguage is returned when the engine has no grammar for the id.
	ErrUnsupportedLanguage = errors.New("unsupported language")
	// ErrSetLanguage is returned when the backend rejects the resolved grammar.
	ErrSetLanguage = errors.New("set language")
	// ErrUnsupportedQuery is returned when the engine cannot run a tree-sitter query.
	ErrUnsupportedQuery = errors.New("unsupported query")
)

// QueryCapture is one named capture from [Engine.Query].
type QueryCapture struct {
	Index     uint32 `json:"index"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	StartByte uint32 `json:"start_byte"`
	EndByte   uint32 `json:"end_byte"`
	Text      string `json:"text,omitempty"`
}

// QueryMatch is one tree-sitter query match from [Engine.Query].
type QueryMatch struct {
	ID           uint32         `json:"id"`
	PatternIndex uint16         `json:"pattern_index"`
	Captures     []QueryCapture `json:"captures"`
}

// Engine parses source into a product CST.
// The entry point puts one on Session via [project.Session.WithEngine].
// ctx is the caller's context. Parse asks the lewkit tree-sitter driver.
type Engine interface {
	// Parse builds a CST for language (pack / as-language id).
	// The returned Tree is a snapshot: no backend lock after return.
	Parse(ctx context.Context, src []byte, language string) (*Tree, error)
	// Has reports whether language can be parsed.
	Has(ctx context.Context, language string) bool
	// Query compiles query and runs it on a live parse of src.
	Query(ctx context.Context, src []byte, language, query string) ([]QueryMatch, error)
}

// CheckLanguage reports ErrUnsupportedLanguage when language is empty.
func CheckLanguage(language string) error {
	if language == "" {
		return fmt.Errorf("%w: empty language", ErrUnsupportedLanguage)
	}
	return nil
}
