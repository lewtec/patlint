package pattern

import (
	"fmt"
	"strings"
)

// CoerceOptions is reserved for CoercePattern / CoerceMatcher call sites.
type CoerceOptions struct{}

// CoercePattern turns a JSON/YAML-decoded pattern value into Core Pat.
//
//	string  → ParseToPat (sexpr)
//	[]any   → DecodePat (list sexpr)
func CoercePattern(v any, opts CoerceOptions) (Pat, error) {
	_ = opts
	if v == nil {
		return nil, fmt.Errorf("%w: pattern: missing", ErrCoerce)
	}
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil, fmt.Errorf("%w: pattern: empty", ErrCoerce)
		}
		return ParseToPat(s)
	case []any:
		expanded, err := Expand(x, nil)
		if err != nil {
			return nil, err
		}
		return DecodePat(expanded)
	default:
		return nil, fmt.Errorf("%w: pattern: want string or array, got %T", ErrCoerce, v)
	}
}

// CoercePatternToNode is CoercePattern then PatToNode.
func CoercePatternToNode(v any, opts CoerceOptions) (Node, error) {
	p, err := CoercePattern(v, opts)
	if err != nil {
		return Node{}, err
	}
	n, err := PatToNode(p)
	if err != nil {
		return Node{}, fmt.Errorf("pattern: pat→node: %w", err)
	}
	return n, nil
}

// PatternValueEmpty reports whether a polymorphic pattern field is unset.
func PatternValueEmpty(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}
