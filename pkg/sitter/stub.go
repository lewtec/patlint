package sitter

import (
	"bytes"
	"context"
	"fmt"
)

// StubLanguage is the only language id [Stub] accepts.
const StubLanguage = "stub"

// Stub is a second Engine that does not use tree-sitter.
// It exists to prove [Engine] is implementable without a grammar backend.
//
// Source is wrapped as:
//
//	source_file
//	  name: ident   (bytes before the first '=')
//	  body: content (rest, including '=')
//
// A source with no '=' is a source_file with one nameless content child.
type Stub struct{}

var _ Engine = Stub{}

// Has implements [Engine].
func (Stub) Has(ctx context.Context, language string) bool {
	return ctx != nil && language == StubLanguage
}

// Parse implements [Engine].
func (Stub) Parse(ctx context.Context, src []byte, language string) (*Tree, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := CheckLanguage(language); err != nil {
		return nil, err
	}
	if language != StubLanguage {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedLanguage, language)
	}
	n := uint32(len(src))
	root := &Node{typ: "source_file", end: n, named: true}
	eq := bytes.IndexByte(src, '=')
	if eq < 0 {
		root.kids = []Node{{typ: "content", end: n, named: true}}
		root.fields = []string{""}
		return &Tree{Root: root, Language: language}, nil
	}
	u := uint32(eq)
	root.kids = []Node{
		{typ: "ident", end: u, named: true},
		{typ: "content", start: u, end: n, named: true},
	}
	root.fields = []string{"name", "body"}
	return &Tree{Root: root, Language: language}, nil
}

// Query implements [Engine]. Stub has no query backend.
func (Stub) Query(context.Context, []byte, string, string) ([]QueryMatch, error) {
	return nil, fmt.Errorf("%w: stub", ErrUnsupportedQuery)
}
