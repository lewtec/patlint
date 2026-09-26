package pattern

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// DecodePatJSON unmarshals a JSON sexpr (lispjson-style array/atoms) into Core Pat.
// Inverse of FormatSexpJSON / patToJSON. See SPEC.md Pattern algebra §11.
func DecodePatJSON(raw []byte) (Pat, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("pattern sexpr: %w", err)
	}
	return DecodePat(v)
}

// DecodePat converts a JSON-decoded value (string, number, []any, …) into Core Pat.
func DecodePat(v any) (Pat, error) {
	p, err := decodePat(v)
	if err != nil {
		return nil, err
	}
	if err := CheckPat(p); err != nil {
		return nil, err
	}
	return p, nil
}

func decodePat(v any) (Pat, error) {
	switch x := v.(type) {
	case nil:
		return nil, fmt.Errorf("%w: pattern sexpr: nil", ErrParse)
	case string:
		// Keywords (sexp bare symbols); other strings are lit token text.
		// Explicit (token "any") / (lit "any") still available for the token "any".
		switch x {
		case "any":
			return Any{}, nil
		case "rest":
			return Rest(), nil
		default:
			return Lit{Text: x}, nil
		}
	case float64:
		// JSON numbers as lit text (e.g. 2).
		if x == float64(int64(x)) {
			return Lit{Text: strconv.FormatInt(int64(x), 10)}, nil
		}
		return Lit{Text: strconv.FormatFloat(x, 'g', -1, 64)}, nil
	case bool:
		return Lit{Text: strconv.FormatBool(x)}, nil
	case []any:
		if len(x) == 0 {
			return nil, fmt.Errorf("%w: pattern sexpr: empty list", ErrParse)
		}
		head, ok := x[0].(string)
		if !ok {
			return nil, fmt.Errorf("%w: pattern sexpr: list head must be string, got %T", ErrParse, x[0])
		}
		return decodeList(head, x[1:])
	default:
		return nil, fmt.Errorf("%w: pattern sexpr: unsupported type %T", ErrParse, v)
	}
}

func decodeList(head string, args []any) (Pat, error) {
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
		s, err := asString(args[0])
		if err != nil {
			return nil, err
		}
		return Token{Text: s}, nil
	case "regex":
		// ["regex", "re"] | ["regex", "re", N] | ["regex", "re", N, "from"] | ["regex", "re", "from"]
		if len(args) < 1 || len(args) > 3 {
			return nil, fmt.Errorf("%w: pattern sexpr: regex wants 1–3 args, got %d", ErrParse, len(args))
		}
		s, err := asString(args[0])
		if err != nil {
			return nil, err
		}
		rx := Regex{RE: s}
		if len(args) >= 2 {
			if n, err := asInt(args[1]); err == nil {
				rx.CaptureGroup = n
				if len(args) == 3 {
					fc, err := asString(args[2])
					if err != nil {
						return nil, err
					}
					rx.FromCapture = fc
				}
			} else {
				if len(args) != 2 {
					return nil, fmt.Errorf("%w: pattern sexpr: regex optional 2nd arg is group int or from-capture string", ErrParse)
				}
				fc, err := asString(args[1])
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
		s, err := asString(args[0])
		if err != nil {
			return nil, err
		}
		return Regex{Equals: s}, nil
	case "ref":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: ref needs 1 arg", ErrParse)
		}
		s, err := asString(args[0])
		if err != nil {
			return nil, err
		}
		return Ref{Target: s}, nil
	case "not":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: not needs 1 arg", ErrParse)
		}
		inner, err := decodePat(args[0])
		if err != nil {
			return nil, err
		}
		return Not{Inner: inner}, nil
	case "seq":
		items, err := decodePatList(args)
		if err != nil {
			return nil, err
		}
		return Seq{Items: items}, nil
	case "alt":
		items, err := decodePatList(args)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("%w: pattern sexpr: empty alt", ErrParse)
		}
		return Alt{Items: items}, nil
	case "?":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: ? needs 1 body", ErrParse)
		}
		body, err := decodePat(args[0])
		if err != nil {
			return nil, err
		}
		return Rep{LocalOptional: true, Min: 0, Max: 1, Body: body}, nil
	case "*":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: * needs 1 body", ErrParse)
		}
		body, err := decodePat(args[0])
		if err != nil {
			return nil, err
		}
		return Rep{Min: 0, Max: -1, Body: body}, nil
	case "+":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: + needs 1 body", ErrParse)
		}
		body, err := decodePat(args[0])
		if err != nil {
			return nil, err
		}
		return Rep{Min: 1, Max: -1, Body: body}, nil
	case "rep":
		return decodeRep(args)
	case "group":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: group needs 1 body", ErrParse)
		}
		body, err := decodePat(args[0])
		if err != nil {
			return nil, err
		}
		return Group{Body: body}, nil
	case "assert_not_behind":
		if len(args) != 1 {
			return nil, fmt.Errorf("%w: pattern sexpr: assert_not_behind needs 1 body", ErrParse)
		}
		body, err := decodePat(args[0])
		if err != nil {
			return nil, err
		}
		return AssertNotBehind{Body: body}, nil
	case "capture":
		if len(args) != 2 {
			return nil, fmt.Errorf("%w: pattern sexpr: capture needs name and body", ErrParse)
		}
		name, err := asString(args[0])
		if err != nil {
			return nil, err
		}
		body, err := decodePat(args[1])
		if err != nil {
			return nil, err
		}
		return Capture{Name: name, Body: body}, nil
	case "unify":
		if len(args) != 2 {
			return nil, fmt.Errorf("%w: pattern sexpr: unify needs name and body", ErrParse)
		}
		name, err := asString(args[0])
		if err != nil {
			return nil, err
		}
		body, err := decodePat(args[1])
		if err != nil {
			return nil, err
		}
		return Unify{Name: name, Body: body}, nil
	default:
		return nil, fmt.Errorf("%w: pattern sexpr: unknown head %q", ErrParse, head)
	}
}

func decodeRep(args []any) (Pat, error) {
	// ["rep", k, body] only — fixed contiguous count (after Expand).
	// Use (* body) / (+ body) / (? body) for Kleene / optional; (rep * body) is illegal.
	if len(args) < 2 {
		return nil, fmt.Errorf("%w: pattern sexpr: rep needs N and body", ErrParse)
	}
	body, err := decodePat(args[len(args)-1])
	if err != nil {
		return nil, err
	}
	qargs := args[:len(args)-1]
	if len(qargs) == 1 {
		if s, ok := qargs[0].(string); ok {
			switch s {
			case "*", "+", "?":
				return nil, fmt.Errorf("%w: pattern sexpr: (rep %s body) is forbidden; use (%s body)", ErrParse, s, s)
			}
		}
		min, err := asInt(qargs[0])
		if err != nil {
			return nil, err
		}
		return Rep{Min: min, Max: min, Body: body}, nil
	}
	return nil, fmt.Errorf("%w: pattern sexpr: (rep N body) only — not (rep N M body); use (* p) / (? p) / (+ p) for other quantifiers", ErrParse)
}

func decodePatList(args []any) ([]Pat, error) {
	out := make([]Pat, 0, len(args))
	for i, a := range args {
		p, err := decodePat(a)
		if err != nil {
			return nil, fmt.Errorf("arg %d: %w", i, err)
		}
		out = append(out, p)
	}
	return out, nil
}

func asString(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: want string, got %T", ErrParse, v)
	}
	return s, nil
}

func asInt(v any) (int, error) {
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("%w: want integer, got %v", ErrParse, n)
		}
		return int(n), nil
	case int:
		return n, nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	case string:
		// Sexp data path: bare 1 is scanned as symbol "1".
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("%w: want integer, got %q", ErrParse, n)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("%w: want number, got %T", ErrParse, v)
	}
}
