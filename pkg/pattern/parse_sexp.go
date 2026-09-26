package pattern

import (
	"fmt"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/lewtec/lewkit/x/text"
)

// ParseSexp parses Lisp-style sexpr text: data → Expand (§10) → Core Pat.
// Example: (seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) ")")
// Supports (let …) / (progn (def …) …) before Core heads.
func ParseSexp(s string) (Pat, error) {
	data, err := ParseSexpData(s)
	if err != nil {
		return nil, err
	}
	expanded, err := Expand(data, nil)
	if err != nil {
		return nil, err
	}
	return DecodePat(expanded)
}

type sexpParser struct {
	s string
	i int
}

func (p *sexpParser) done() bool { return p.i >= len(p.s) }
func (p *sexpParser) rest() string {
	if p.done() {
		return ""
	}
	return p.s[p.i:]
}
func (p *sexpParser) peek() byte {
	if p.done() {
		return 0
	}
	return p.s[p.i]
}
func (p *sexpParser) skipSpace() {
	for p.i < len(p.s) {
		r, w := utf8.DecodeRuneInString(p.s[p.i:])
		if !unicode.IsSpace(r) {
			return
		}
		p.i += w
	}
}

// loc is 1-based line and column at p.i.
func (p *sexpParser) loc() (line, col int) {
	off := p.i
	if off < 0 {
		off = 0
	}
	if off > len(p.s) {
		off = len(p.s)
	}
	li := text.NewLineIndex(p.s)
	line, col0 := li.LineColumnAt(off)
	return line, col0 + 1
}

// parseForm reads one atom or list.
func (p *sexpParser) parseForm() (Pat, error) {
	p.skipSpace()
	if p.done() {
		line, col := p.loc()
		return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: unexpected end of input", ErrParse, line, col)
	}
	switch p.peek() {
	case '(':
		return p.parseList()
	case '"':
		s, err := p.scanString()
		if err != nil {
			return nil, err
		}
		return Lit{Text: s}, nil
	default:
		sym, err := p.scanSymbol()
		if err != nil {
			return nil, err
		}
		return symbolToPat(sym)
	}
}

func symbolToPat(sym string) (Pat, error) {
	switch sym {
	case "any":
		return Any{}, nil
	case "rest":
		return Rest(), nil
	default:
		// Bare unquoted symbol as lit (rare; FormatSexp quotes lits).
		return Lit{Text: sym}, nil
	}
}

func (p *sexpParser) parseList() (Pat, error) {
	if p.peek() != '(' {
		line, col := p.loc()
		return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: expected '('", ErrParse, line, col)
	}
	p.i++ // (
	p.skipSpace()
	if p.peek() == ')' {
		p.i++
		line, col := p.loc()
		return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: empty list", ErrParse, line, col)
	}

	// Head is a symbol (or string for invalid).
	head, err := p.scanListHead()
	if err != nil {
		return nil, err
	}
	var args []any // mixed Pat and raw symbols/numbers for rep quant
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
		// For list heads that need raw atoms (rep quant, capture name), parse loosely.
		arg, err := p.parseListArg()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	return decodeListFromSexp(head, args)
}

// parseListArg returns either a Pat or a raw string atom (symbol / quoted / number-like).
func (p *sexpParser) parseListArg() (any, error) {
	p.skipSpace()
	if p.done() {
		line, col := p.loc()
		return nil, fmt.Errorf("%w: pattern sexpr:%d:%d: unexpected end", ErrParse, line, col)
	}
	switch p.peek() {
	case '(':
		return p.parseList()
	case '"':
		s, err := p.scanString()
		if err != nil {
			return nil, err
		}
		return s, nil // bare string atom for decodeList (ref targets, etc.)
	default:
		sym, err := p.scanSymbol()
		if err != nil {
			return nil, err
		}
		// Prefer known unit pats for nested body positions; raw string for names/quants.
		return sym, nil
	}
}

func (p *sexpParser) scanListHead() (string, error) {
	p.skipSpace()
	if p.peek() == '"' {
		line, col := p.loc()
		return "", fmt.Errorf("%w: pattern sexpr:%d:%d: list head must be a symbol", ErrParse, line, col)
	}
	if p.peek() == '(' {
		line, col := p.loc()
		return "", fmt.Errorf("%w: pattern sexpr:%d:%d: list head must be a symbol", ErrParse, line, col)
	}
	return p.scanSymbol()
}

func (p *sexpParser) scanString() (string, error) {
	if p.peek() != '"' {
		line, col := p.loc()
		return "", fmt.Errorf("%w: pattern sexpr:%d:%d: expected string", ErrParse, line, col)
	}
	// Use Go strconv unquoting on a full quoted span.
	start := p.i
	p.i++ // "
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '\\' {
			p.i++
			if p.i < len(p.s) {
				p.i++
			}
			continue
		}
		if c == '"' {
			p.i++
			raw := p.s[start:p.i]
			s, err := strconv.Unquote(raw)
			if err != nil {
				line, col := p.loc()
				return "", fmt.Errorf("%w: pattern sexpr:%d:%d: bad string: %v", ErrParse, line, col, err)
			}
			return s, nil
		}
		p.i++
	}
	line, col := p.loc()
	return "", fmt.Errorf("%w: pattern sexpr:%d:%d: unterminated string", ErrParse, line, col)
}

func (p *sexpParser) scanSymbol() (string, error) {
	if p.done() {
		line, col := p.loc()
		return "", fmt.Errorf("%w: pattern sexpr:%d:%d: expected symbol", ErrParse, line, col)
	}
	start := p.i
	for p.i < len(p.s) {
		r, w := utf8.DecodeRuneInString(p.s[p.i:])
		if unicode.IsSpace(r) || r == '(' || r == ')' || r == '"' {
			break
		}
		p.i += w
	}
	if p.i == start {
		line, col := p.loc()
		return "", fmt.Errorf("%w: pattern sexpr:%d:%d: expected symbol", ErrParse, line, col)
	}
	return p.s[start:p.i], nil
}

// decodeListFromSexp builds Pat from head + args where args are Pat or string atoms.
func decodeListFromSexp(head string, args []any) (Pat, error) {
	// Convert string atoms that are unit keywords to Pat where decodeList expects Pat children.
	// decodeList expects []any of JSON shapes; we adapt.

	switch head {
	case "any":
		if len(args) != 0 {
			return nil, fmt.Errorf("%w: pattern sexpr: any takes no args", ErrParse)
		}
		return Any{}, nil
	case "rest":
		if len(args) != 0 {
			return nil, fmt.Errorf("%w: pattern sexpr: rest takes no args", ErrParse)
		}
		return Rest(), nil
	case "token":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: token needs 1 arg", ErrParse)
		}
		s, err := atomString(args[0])
		if err != nil {
			return nil, err
		}
		return Token{Text: s}, nil
	case "regex":
		// (regex "re") | (regex "re" N) | (regex "re" N "fromCapture") | (regex "re" "fromCapture")
		// N = CaptureGroup; optional trailing string = FromCapture (rewrite stretch).
		if len(args) < 1 || len(args) > 3 {
			return nil, fmt.Errorf("%w: pattern sexpr: regex wants 1–3 args, got %d", ErrParse, len(args))
		}
		s, err := atomString(args[0])
		if err != nil {
			return nil, err
		}
		rx := Regex{RE: s}
		if len(args) >= 2 {
			if n, err := atomInt(args[1]); err == nil {
				rx.CaptureGroup = n
				if len(args) == 3 {
					fc, err := atomString(args[2])
					if err != nil {
						return nil, err
					}
					rx.FromCapture = fc
				}
			} else {
				if len(args) != 2 {
					return nil, fmt.Errorf("%w: pattern sexpr: regex optional 2nd arg is group int or from-capture string", ErrParse)
				}
				fc, err := atomString(args[1])
				if err != nil {
					return nil, err
				}
				rx.FromCapture = fc
			}
		}
		return rx, nil
	case "equals":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: equals needs 1 arg", ErrParse)
		}
		s, err := atomString(args[0])
		if err != nil {
			return nil, err
		}
		return Regex{Equals: s}, nil
	case "ref":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: ref needs 1 arg", ErrParse)
		}
		s, err := atomString(args[0])
		if err != nil {
			return nil, err
		}
		return Ref{Target: s}, nil
	case "not":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: not needs 1 arg", ErrParse)
		}
		inner, err := argToPat(args[0])
		if err != nil {
			return nil, err
		}
		return Not{Inner: inner}, nil
	case "seq":
		items, err := argsToPats(args)
		if err != nil {
			return nil, err
		}
		return Seq{Items: items}, nil
	case "alt":
		items, err := argsToPats(args)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("%w: pattern sexpr: empty alt", ErrParse)
		}
		return Alt{Items: items}, nil
	case "?", "*", "+":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: %s needs 1 body", ErrParse, head)
		}
		body, err := argToPat(args[0])
		if err != nil {
			return nil, err
		}
		switch head {
		case "?":
			return Rep{LocalOptional: true, Min: 0, Max: 1, Body: body}, nil
		case "*":
			return Rep{Min: 0, Max: -1, Body: body}, nil
		default:
			return Rep{Min: 1, Max: -1, Body: body}, nil
		}
	case "rep":
		return decodeRepSexp(args)
	case "group":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: group needs 1 body", ErrParse)
		}
		body, err := argToPat(args[0])
		if err != nil {
			return nil, err
		}
		return Group{Body: body}, nil
	case "assert_not_behind":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: assert_not_behind needs 1 body", ErrParse)
		}
		body, err := argToPat(args[0])
		if err != nil {
			return nil, err
		}
		return AssertNotBehind{Body: body}, nil
	case "capture", "unify":
		if len(args) != 2 {
			return nil, fmt.Errorf("%w: pattern sexpr: %s needs name and body", ErrParse, head)
		}
		name, err := atomString(args[0])
		if err != nil {
			return nil, err
		}
		body, err := argToPat(args[1])
		if err != nil {
			return nil, err
		}
		if head == "capture" {
			return Capture{Name: name, Body: body}, nil
		}
		return Unify{Name: name, Body: body}, nil
	default:
		return nil, fmt.Errorf("%w: pattern sexpr: unknown head %q", ErrParse, head)
	}
}

func decodeRepSexp(args []any) (Pat, error) {
	// (rep N body) only — fixed count. (* body) / (+ body) / (? body) for other quantifiers.
	if len(args) != 2 {
		return nil, fmt.Errorf("%w: pattern sexpr: (rep N body) only, got %d args", ErrParse, len(args))
	}
	if s, err := atomString(args[0]); err == nil {
		switch s {
		case "*", "+", "?":
			return nil, fmt.Errorf("%w: pattern sexpr: (rep %s body) is forbidden; use (%s body)", ErrParse, s, s)
		}
	}
	n, err := atomInt(args[0])
	if err != nil {
		return nil, fmt.Errorf("%w: pattern sexpr: rep N: %v", ErrParse, err)
	}
	if n < 0 {
		return nil, fmt.Errorf("%w: pattern sexpr: rep count %d invalid", ErrParse, n)
	}
	body, err := argToPat(args[1])
	if err != nil {
		return nil, err
	}
	return Rep{Min: n, Max: n, Body: body}, nil
}

func atomString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case Pat:
		// FormatSexp may produce lit as quoted form parsed as Lit
		if lit, ok := x.(Lit); ok {
			return lit.Text, nil
		}
		return "", fmt.Errorf("%w: pattern sexpr: want string atom, got %T", ErrParse, v)
	default:
		return "", fmt.Errorf("%w: pattern sexpr: want string atom, got %T", ErrParse, v)
	}
}

// atomInt parses an integer atom (sexpr symbol "1" or JSON number).
func atomInt(v any) (int, error) {
	switch x := v.(type) {
	case string:
		n, err := strconv.Atoi(x)
		if err != nil {
			return 0, err
		}
		return n, nil
	case float64:
		if x != float64(int(x)) {
			return 0, fmt.Errorf("%w: want integer, got %v", ErrParse, x)
		}
		return int(x), nil
	case int:
		return x, nil
	default:
		return 0, fmt.Errorf("%w: want integer atom, got %T", ErrParse, v)
	}
}

func argToPat(v any) (Pat, error) {
	switch x := v.(type) {
	case Pat:
		return x, nil
	case string:
		return symbolToPat(x)
	default:
		return nil, fmt.Errorf("%w: pattern sexpr: want form, got %T", ErrParse, v)
	}
}

func argsToPats(args []any) ([]Pat, error) {
	out := make([]Pat, 0, len(args))
	for i, a := range args {
		// seq args: strings are lits (quoted in FormatSexp become string atoms; unquoted symbols → lit/any)
		switch x := a.(type) {
		case Pat:
			out = append(out, x)
		case string:
			// In seq, bare "any"/"rest" are units; other symbols are lits (FormatSexp quotes real lits as strings).
			p, err := symbolToPat(x)
			if err != nil {
				return nil, fmt.Errorf("arg %d: %w", i, err)
			}
			// symbolToPat maps unknown to Lit — but quoted strings arrive as string too.
			// Quoted "(" arrives as string "(" → Lit via... symbolToPat would make Lit{"("} only for unquoted.
			// Quoted scan returns string in parseListArg without wrapping Lit — for seq we need Lit{Text: s}.
			if x == "any" || x == "rest" {
				out = append(out, p)
			} else {
				out = append(out, Lit{Text: x})
			}
		default:
			return nil, fmt.Errorf("%w: arg %d: want form, got %T", ErrParse, i, a)
		}
	}
	return out, nil
}
