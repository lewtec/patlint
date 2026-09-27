package pattern

import (
	"fmt"
	"unicode/utf8"
)

// ParseSexpData parses Lisp sexpr text into data forms: strings (atoms) and []any (lists).
// Does not interpret Core/Matcher heads — use Expand then DecodePat / matcherFromList.
func ParseSexpData(s string) (any, error) {
	forms, err := ParseSexpDataFile(s)
	if err != nil {
		return nil, err
	}
	if len(forms) != 1 {
		return nil, fmt.Errorf("%w: pattern sexpr: want one form, got %d", ErrParse, len(forms))
	}
	return forms[0], nil
}

// ParseSexpDataFile parses zero or more top-level sexpr forms, skipping ;; line comments.
func ParseSexpDataFile(s string) ([]any, error) {
	p := &sexpParser{s: s}
	var forms []any
	for {
		p.skipSpaceAndComments()
		if p.done() {
			break
		}
		v, err := p.parseDataForm()
		if err != nil {
			return nil, err
		}
		forms = append(forms, v)
	}
	return forms, nil
}

func (p *sexpParser) skipSpaceAndComments() {
	for {
		p.skipSpace()
		if p.done() {
			return
		}
		// ;; line comment
		if p.i+1 < len(p.s) && p.s[p.i] == ';' && p.s[p.i+1] == ';' {
			for p.i < len(p.s) {
				r, w := utf8.DecodeRuneInString(p.s[p.i:])
				p.i += w
				if r == '\n' {
					break
				}
			}
			continue
		}
		// single ; comment to EOL (common lisp style), if present
		if p.s[p.i] == ';' {
			for p.i < len(p.s) {
				r, w := utf8.DecodeRuneInString(p.s[p.i:])
				p.i += w
				if r == '\n' {
					break
				}
			}
			continue
		}
		return
	}
}

func (p *sexpParser) parseDataForm() (any, error) {
	p.skipSpaceAndComments()
	if p.done() {
		line, col := p.loc()
		return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: unexpected end of input", ErrParse, line, col)
	}
	switch p.peek() {
	case '(':
		return p.parseDataList()
	case '"':
		return p.scanString()
	default:
		return p.scanSymbol()
	}
}

func (p *sexpParser) parseDataList() (any, error) {
	if p.peek() != '(' {
		line, col := p.loc()
		return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: expected '('", ErrParse, line, col)
	}
	p.i++ // (
	var elems []any
	for {
		p.skipSpaceAndComments()
		if p.done() {
			line, col := p.loc()
			return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: unclosed list", ErrParse, line, col)
		}
		if p.peek() == ')' {
			p.i++
			break
		}
		el, err := p.parseDataForm()
		if err != nil {
			return nil, err
		}
		elems = append(elems, el)
	}
	return elems, nil
}
