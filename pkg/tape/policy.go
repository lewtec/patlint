package tape

// Policy controls which tree-sitter nodes become single tape cells.
// Zero Policy uses DefaultPolicy behavior for nil hooks.
type Policy struct {
	// AtomicType, when non-nil, reports node types that must not be recursed
	// into (the whole node is one cell). Default: common string-literal types.
	AtomicType func(typ string) bool
	// AtomicSpan, when non-nil, reports source spans kept as one cell even when
	// the node has children (language composites). Default: none. Packs supply
	// texts via WithAtomicTexts.
	AtomicSpan func(src []byte, start, end uint32) bool
}

// DefaultPolicy returns the language-agnostic leaf policy: common string
// literal node types are atomic; no composite spans.
func DefaultPolicy() Policy {
	return Policy{
		AtomicType: defaultAtomicType,
	}
}

// WithAtomicTexts keeps each exact source slice as one tape cell.
func WithAtomicTexts(texts []string) Policy {
	p := DefaultPolicy()
	if len(texts) == 0 {
		return p
	}
	set := make(map[string]struct{}, len(texts))
	for _, t := range texts {
		if t != "" {
			set[t] = struct{}{}
		}
	}
	if len(set) == 0 {
		return p
	}
	p.AtomicSpan = func(src []byte, start, end uint32) bool {
		if end < start || int(end) > len(src) {
			return false
		}
		_, ok := set[string(src[start:end])]
		return ok
	}
	return p
}

func (p Policy) atomicType(typ string) bool {
	if p.AtomicType != nil {
		return p.AtomicType(typ)
	}
	return defaultAtomicType(typ)
}

func (p Policy) atomicSpan(src []byte, start, end uint32) bool {
	if p.AtomicSpan == nil {
		return false
	}
	return p.AtomicSpan(src, start, end)
}

// usesDefaultAtomicTypes is true when AtomicType is unset or is exactly
// defaultAtomicType (DefaultPolicy). Then collectLeaves may skip Type() on
// non-quoted internal nodes. Custom AtomicType closures take the slow path.
func (p Policy) usesDefaultAtomicTypes() bool {
	if p.AtomicType == nil {
		return true
	}
	// Func values are comparable when both refer to the same package function.
	return equalDefaultAtomicType(p.AtomicType)
}

// equalDefaultAtomicType avoids an inline == that some go versions warn about.
func equalDefaultAtomicType(f func(string) bool) bool {
	return f != nil && f("interpreted_string_literal") && f("raw_string_literal") &&
		f("string_literal") && f("string") && f("template_string") &&
		!f("identifier") && !f("func") && !f("call_expression")
}

func defaultAtomicType(typ string) bool {
	switch typ {
	case "interpreted_string_literal",
		"raw_string_literal",
		"string_literal",
		"string",
		"template_string",
		"str_lit": // commonlisp / .rft
		return true
	default:
		return false
	}
}
