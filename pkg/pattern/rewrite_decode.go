package pattern

import "fmt"

// peelRewrite records emit/take/lang and returns the match tree.
func peelRewrite(v any, emit *any, take, lang *string) (any, error) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return v, nil
	}
	head, _ := list[0].(string)
	switch head {
	case "rule", "builtin", "def":
		return nil, fmt.Errorf("%s only allowed at script top level (not nested in match/rewrite bodies)", head)
	case "rewrite":
		if len(list) != 3 {
			return nil, fmt.Errorf("rewrite wants match and emit")
		}
		if *emit != nil {
			return nil, fmt.Errorf("nested rewrite")
		}
		*emit = list[2]
		return peelRewrite(list[1], emit, take, lang)
	case "take":
		if len(list) != 3 {
			return nil, fmt.Errorf("take wants name and match")
		}
		name, ok := list[1].(string)
		if !ok {
			return nil, fmt.Errorf("take name must be a string")
		}
		if *take != "" {
			return nil, fmt.Errorf("nested take")
		}
		*take = name
		return peelRewrite(list[2], emit, take, lang)
	case "lang":
		if len(list) != 3 {
			return nil, fmt.Errorf("lang wants id and body")
		}
		id, ok := list[1].(string)
		if !ok {
			return nil, fmt.Errorf("lang id must be a string")
		}
		if *lang != "" && *lang != id {
			return nil, fmt.Errorf("conflicting lang")
		}
		*lang = id
		return peelRewrite(list[2], emit, take, lang)
	case "under":
		if len(list) < 3 {
			return nil, fmt.Errorf("under needs scope and body")
		}
		scope, body := list[1], list[2]
		if sl, ok := scope.([]any); ok && len(sl) >= 2 {
			if sh, _ := sl[0].(string); sh == "lang" {
				id, ok := sl[1].(string)
				if !ok {
					return nil, fmt.Errorf("lang id must be a string")
				}
				if *lang != "" && *lang != id {
					return nil, fmt.Errorf("conflicting lang")
				}
				*lang = id
				return peelRewrite(body, emit, take, lang)
			}
		}
		body2, err := peelRewrite(body, emit, take, lang)
		if err != nil {
			return nil, err
		}
		return []any{"under", scope, body2}, nil
	default:
		return v, nil
	}
}

func opFromExpanded(v any) (Op, bool, error) {
	var emit any
	var take, lang string
	match, err := peelRewrite(v, &emit, &take, &lang)
	if err != nil {
		return Op{}, false, err
	}
	if emit == nil {
		return Op{}, false, nil
	}
	m, err := MatcherFromExpanded(match)
	if err != nil {
		return Op{}, false, err
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		return Op{}, false, err
	}
	node, err := matcherPatternNode(cm)
	if err != nil {
		return Op{}, false, err
	}
	return Op{
		Mode:      "rewrite",
		Lang:      lang,
		Matcher:   cm,
		PatternIR: node,
		Emit:      emit,
		Take:      take,
	}, true, nil
}

func (vm *LispVM) rewriteOps() ([]Op, error) {
	if vm == nil {
		return nil, fmt.Errorf("%w: lispvm: nil", ErrExtract)
	}
	var out []Op
	for _, f := range vm.ExtraForms() {
		var body any
		switch f.Head {
		case "def", "builtin":
			continue
		case "rule":
			list, ok := f.Data.([]any)
			if !ok || len(list) != 5 {
				continue
			}
			body = list[4]
		default:
			body = f.Data
		}
		op, ok, err := opFromExpanded(body)
		if err != nil {
			return nil, fmt.Errorf("%s: form %d: %w", f.File, f.Index, err)
		}
		if !ok {
			continue
		}
		out = append(out, op)
	}
	return out, nil
}
