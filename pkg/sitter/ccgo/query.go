package ccgo

import (
	"context"
	"fmt"

	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar"
)

// Query implements [sitter.Engine].
// The lewkit driver has no query API yet, so this still uses the ccgo grammar runtime.
func (Engine) Query(ctx context.Context, src []byte, language, query string) ([]sitter.QueryMatch, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := sitter.CheckLanguage(language); err != nil {
		return nil, err
	}
	lang, ok := grammar.Get(language)
	if !ok {
		return nil, fmt.Errorf("%w: %q", sitter.ErrUnsupportedLanguage, language)
	}
	p := grammar.NewParser()
	if !p.SetLanguage(lang) {
		p.Delete()
		return nil, fmt.Errorf("%w: %q", sitter.ErrSetLanguage, language)
	}
	gt := p.ParseBytes(src)
	defer p.Delete()
	defer gt.Delete()
	q, err := grammar.NewQuery(lang, query)
	if err != nil {
		return nil, err
	}
	defer q.Delete()
	raw := q.ExecuteMatches(gt.RootNode(), src)
	out := make([]sitter.QueryMatch, len(raw))
	for i, m := range raw {
		caps := make([]sitter.QueryCapture, len(m.Captures))
		for j, c := range m.Captures {
			caps[j] = sitter.QueryCapture{
				Index:     c.Index,
				Name:      c.Name,
				Type:      c.Type,
				StartByte: c.StartByte,
				EndByte:   c.EndByte,
				Text:      c.Text,
			}
		}
		out[i] = sitter.QueryMatch{ID: m.ID, PatternIndex: m.PatternIndex, Captures: caps}
	}
	return out, nil
}
