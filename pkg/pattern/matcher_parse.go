package pattern

import (
	"fmt"
	"strings"
)

// ParseMatcher parses a matcher sexpr. Matcher heads: path, under, and, or, not.
// Anything else is a Core Pat leaf.
func ParseMatcher(s string) (Matcher, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%w: matcher: empty", ErrMatcher)
	}
	data, err := ParseSexpData(s)
	if err != nil {
		return nil, err
	}
	expanded, err := Expand(data, nil)
	if err != nil {
		return nil, err
	}
	return matcherFromExpanded(expanded)
}

// CoerceMatcher turns a YAML/JSON pattern value into Matcher.
func CoerceMatcher(v any, opts CoerceOptions) (Matcher, error) {
	if v == nil {
		return nil, fmt.Errorf("%w: matcher: missing", ErrMatcher)
	}
	switch x := v.(type) {
	case string:
		return ParseMatcher(x)
	case []any:
		expanded, err := Expand(x, nil)
		if err != nil {
			return nil, err
		}
		return matcherFromExpanded(expanded)
	default:
		return nil, fmt.Errorf("%w: matcher: want string or array, got %T", ErrMatcher, v)
	}
}

// matcherFromExpanded builds a Matcher from post-Expand data.
func matcherFromExpanded(v any) (Matcher, error) {
	switch x := v.(type) {
	case []any:
		return matcherFromList(x)
	case string:
		// Bare atom after expand (e.g. only a bound leaf form that was a symbol)
		pat, err := DecodePat(x)
		if err != nil {
			return nil, err
		}
		return LeafM{Pat: pat}, nil
	default:
		pat, err := DecodePat(x)
		if err != nil {
			return nil, fmt.Errorf("matcher: %w", err)
		}
		return LeafM{Pat: pat}, nil
	}
}

func (p *sexpParser) parseMatcherForm() (Matcher, error) {
	p.skipSpace()
	if p.done() {
		line, col := p.loc()
		return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: unexpected end", ErrParse, line, col)
	}
	if p.peek() != '(' {
		// bare atom → Core symbol/lit leaf
		pat, err := p.parseForm()
		if err != nil {
			return nil, err
		}
		return LeafM{Pat: pat}, nil
	}
	// Look at head without consuming whole list twice: parse list head + args.
	if p.peek() != '(' {
		line, col := p.loc()
		return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: expected '('", ErrParse, line, col)
	}
	p.i++ // (
	p.skipSpace()
	if p.peek() == ')' {
		line, col := p.loc()
		return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: empty list", ErrParse, line, col)
	}
	head, err := p.scanListHead()
	if err != nil {
		return nil, err
	}
	switch head {
	case "path", "under", "and", "or", "not", "take", "node", "as-language":
		var args []any
		for {
			p.skipSpace()
			if p.done() {
				line, col := p.loc()
				return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: unclosed list", ErrParse, line, col)
			}
			if p.peek() == ')' {
				p.i++
				break
			}
			arg, err := p.parseMatcherListArg(head)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
		return decodeMatcherList(head, args)
	default:
		// Rewind is hard; re-parse full list as Core Pat from remaining.
		// We already consumed '(' and head — reconstruct via remaining args as Pat.
		var args []any
		for {
			p.skipSpace()
			if p.done() {
				line, col := p.loc()
				return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: unclosed list", ErrParse, line, col)
			}
			if p.peek() == ')' {
				p.i++
				break
			}
			arg, err := p.parseListArg()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
		pat, err := decodeListFromSexp(head, args)
		if err != nil {
			return nil, err
		}
		if err := CheckPat(pat); err != nil {
			return nil, err
		}
		return LeafM{Pat: pat}, nil
	}
}

// parseMatcherListArg: path's first arg is glob string; rest are matchers.
func (p *sexpParser) parseMatcherListArg(head string) (any, error) {
	// For path first arg we need a string; parseListArg returns string atoms.
	return p.parseListArgOrMatcher(head)
}

func (p *sexpParser) parseListArgOrMatcher(head string) (any, error) {
	p.skipSpace()
	if p.done() {
		line, col := p.loc()
		return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: unexpected end", ErrParse, line, col)
	}
	if p.peek() == '(' {
		// Nested form: could be matcher or Core pat — use parseMatcherForm
		return p.parseMatcherForm()
	}
	if p.peek() == '"' {
		s, err := p.scanString()
		if err != nil {
			return nil, err
		}
		return s, nil
	}
	sym, err := p.scanSymbol()
	if err != nil {
		return nil, err
	}
	return sym, nil
}

func decodeMatcherList(head string, args []any) (Matcher, error) {
	switch head {
	case "path":
		if len(args) < 1 {
			return nil, fmt.Errorf("%w: matcher: path needs a glob", ErrMatcher)
		}
		glob, err := atomString(args[0])
		if err != nil {
			return nil, fmt.Errorf("matcher: path glob: %w", err)
		}
		if len(args) == 1 {
			return PathM{Glob: glob}, nil
		}
		body, err := andBodies(args[1:])
		if err != nil {
			return nil, err
		}
		return PathM{Glob: glob, Body: body}, nil
	case "under":
		if len(args) < 2 {
			return nil, fmt.Errorf("%w: matcher: under needs region and body", ErrMatcher)
		}
		region, err := argMatcher(args[0])
		if err != nil {
			return nil, err
		}
		body, err := andBodies(args[1:])
		if err != nil {
			return nil, err
		}
		return UnderM{Region: region, Body: body}, nil
	case "take":
		// (take name body) — name is string or symbol; body is a finder.
		if len(args) != 2 {
			return nil, fmt.Errorf("%w: matcher: take wants name and body", ErrMatcher)
		}
		name, err := atomString(args[0])
		if err != nil {
			return nil, fmt.Errorf("matcher: take name: %w", err)
		}
		body, err := argMatcher(args[1])
		if err != nil {
			return nil, err
		}
		return TakeM{Name: name, Body: body}, nil
	case "node":
		// (node TYPE) — region finder by tree-sitter node type.
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: matcher: node wants one type", ErrMatcher)
		}
		typ, err := atomString(args[0])
		if err != nil {
			return nil, fmt.Errorf("matcher: node type: %w", err)
		}
		if typ == "" {
			return nil, fmt.Errorf("%w: matcher: node type empty", ErrMatcher)
		}
		return NodeM{Type: typ}, nil
	case "as-language":
		// (as-language ID REGION BODY) — reparse region spans as ID, run body.
		if len(args) < 3 {
			return nil, fmt.Errorf("%w: matcher: as-language wants lang region body", ErrMatcher)
		}
		lang, err := atomString(args[0])
		if err != nil {
			return nil, fmt.Errorf("matcher: as-language lang: %w", err)
		}
		region, err := argMatcher(args[1])
		if err != nil {
			return nil, err
		}
		body, err := andBodies(args[2:])
		if err != nil {
			return nil, err
		}
		return AsLangM{Lang: lang, Region: region, Body: body}, nil
	case "and":
		items, err := argsMatchers(args)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("%w: matcher: empty and", ErrMatcher)
		}
		return AndM{Items: items}, nil
	case "or":
		items, err := argsMatchers(args)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("%w: matcher: empty or", ErrMatcher)
		}
		return OrM{Items: items}, nil
	case "not":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: matcher: not needs one gate", ErrMatcher)
		}
		inner, err := argMatcher(args[0])
		if err != nil {
			return nil, err
		}
		return NotM{Inner: inner}, nil
	default:
		if head == "rule" || head == "builtin" || head == "def" {
			return nil, fmt.Errorf("%w: matcher: %s only allowed at script top level (not nested in a pattern body)", ErrMatcher, head)
		}
		return nil, fmt.Errorf("%w: matcher: unknown head %q", ErrMatcher, head)
	}
}

func matcherFromList(x []any) (Matcher, error) {
	if len(x) == 0 {
		return nil, fmt.Errorf("%w: matcher: empty list", ErrMatcher)
	}
	head, ok := x[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: matcher: list head must be string, got %T", ErrMatcher, x[0])
	}
	switch head {
	case "path", "under", "and", "or", "not", "take", "node", "as-language":
		// Convert nested []any to Matcher or leave atoms
		args := make([]any, 0, len(x)-1)
		for _, a := range x[1:] {
			args = append(args, normalizeMatcherArg(a))
		}
		return decodeMatcherList(head, args)
	default:
		pat, err := DecodePat(x)
		if err != nil {
			return nil, err
		}
		return LeafM{Pat: pat}, nil
	}
}

func normalizeMatcherArg(a any) any {
	switch t := a.(type) {
	case []any:
		// keep as list; argMatcher / decode will handle
		return t
	case Matcher:
		return t
	default:
		return a
	}
}

func andBodies(args []any) (Matcher, error) {
	items, err := argsMatchers(args)
	if err != nil {
		return nil, err
	}
	if len(items) == 1 {
		return items[0], nil
	}
	return AndM{Items: items}, nil
}

func argsMatchers(args []any) ([]Matcher, error) {
	out := make([]Matcher, 0, len(args))
	for i, a := range args {
		m, err := argMatcher(a)
		if err != nil {
			return nil, fmt.Errorf("arg %d: %w", i, err)
		}
		out = append(out, m)
	}
	return out, nil
}

func argMatcher(a any) (Matcher, error) {
	switch x := a.(type) {
	case Matcher:
		return x, nil
	case Pat:
		return LeafM{Pat: x}, nil
	case string:
		// bare symbol in matcher position — treat as Core lit leaf via symbolToPat
		pat, err := symbolToPat(x)
		if err != nil {
			return nil, err
		}
		return LeafM{Pat: pat}, nil
	case []any:
		return matcherFromList(x)
	default:
		// try DecodePat for nested JSON shapes
		pat, err := DecodePat(x)
		if err != nil {
			return nil, fmt.Errorf("%w: matcher: want form, got %T", ErrMatcher, a)
		}
		return LeafM{Pat: pat}, nil
	}
}
