package pattern

import (
	"fmt"
	"strconv"
)

// Expand applies pure expand-time rules (SPEC.md Pattern algebra §10): let, def/fn, bare
// symbol substitution. Result has no let/def/fn/progn/get left.
// env may be nil. Expand always seeds macros from core.rft (?, +, until);
// caller bindings override prelude. Expand does not mutate env.
func Expand(form any, env expandEnv) (any, error) {
	return expand(form, withExpandPrelude(env), 0)
}

// ExpandSexp parses Lisp sexpr text to data, expands with empty env, returns data.
func ExpandSexp(s string) (any, error) {
	data, err := ParseSexpData(s)
	if err != nil {
		return nil, err
	}
	return Expand(data, nil)
}

type expandEnv map[string]any

type expandFn struct {
	params []string
	body   any
}

const maxExpandDepth = 64

func expand(form any, env expandEnv, depth int) (any, error) {
	if depth > maxExpandDepth {
		return nil, fmt.Errorf("%w: expand: depth limit exceeded", ErrExpand)
	}
	switch x := form.(type) {
	case nil:
		return nil, fmt.Errorf("%w: expand: nil form", ErrExpand)
	case string:
		if v, ok := env[x]; ok {
			// expandFn is only applied as a list head, never bare-substituted
			// (so lit "?" / prelude names stay symbols when not called).
			if _, isFn := v.(expandFn); isFn {
				return x, nil
			}
			return cloneForm(v), nil
		}
		return x, nil
	case []any:
		return expandList(x, env, depth)
	case expandFn:
		// Already a function value (should not appear as free form often).
		return x, nil
	case float64, bool, int:
		return x, nil
	default:
		return nil, fmt.Errorf("%w: expand: unsupported form type %T", ErrExpand, form)
	}
}

func expandList(x []any, env expandEnv, depth int) (any, error) {
	if len(x) == 0 {
		return nil, fmt.Errorf("%w: expand: empty list", ErrExpand)
	}
	head, ok := x[0].(string)
	if !ok {
		return nil, fmt.Errorf("%w: expand: list head must be a symbol, got %T", ErrExpand, x[0])
	}

	switch head {
	case "let":
		return expandLet(x[1:], env, depth)
	case "fn":
		return parseExpandFn(x[1:])
	case "def":
		return nil, fmt.Errorf("%w: expand: def only allowed inside progn (use let + fn, or progn with def)", ErrExpand)
	case "progn":
		return expandProgn(x[1:], env, depth)
	case "get":
		if len(x) != 2 {
			return nil, fmt.Errorf("%w: expand: get needs one name", ErrExpand)
		}
		name, ok := x[1].(string)
		if !ok {
			return nil, fmt.Errorf("%w: expand: get name must be a symbol", ErrExpand)
		}
		v, ok := env[name]
		if !ok {
			return nil, fmt.Errorf("%w: expand: unbound %q", ErrExpand, name)
		}
		return cloneForm(v), nil
	case "rep":
		// (rep N p) contiguous macro only. Not (rep *|?|+ p) — use (* p) / (? p) / (+ p).
		return expandRep(x[1:], env, depth)
	case "*":
		// (* p) — sole Kleene NFA primitive (expand body only).
		if len(x) != 2 {
			return nil, fmt.Errorf("%w: expand: * wants one body, got %d args", ErrExpand, len(x)-1)
		}
		body, err := expand(x[1], env, depth+1)
		if err != nil {
			return nil, err
		}
		return []any{"*", body}, nil
	}
	// Prelude / user expandFns: (? p) (+ p) and any (def …) macros.
	// Checked before unknown-head error; after special forms.

	// Function application: head names an expandFn in env.
	if fn, ok := env[head].(expandFn); ok {
		args := make([]any, 0, len(x)-1)
		for _, a := range x[1:] {
			ea, err := expand(a, env, depth+1)
			if err != nil {
				return nil, err
			}
			args = append(args, ea)
		}
		return applyExpandFn(fn, args, env, depth)
	}

	// Known operator: keep head, expand args (with protected slots).
	if isExpandKnownHead(head) {
		out := make([]any, len(x))
		out[0] = head
		for i := 1; i < len(x); i++ {
			if isProtectedNameSlot(head, i) {
				// Do not env-lookup the name atom; still allow nested forms.
				if s, ok := x[i].(string); ok {
					out[i] = s
					continue
				}
			}
			ea, err := expand(x[i], env, depth+1)
			if err != nil {
				return nil, err
			}
			out[i] = ea
		}
		return out, nil
	}

	// Unknown head: expand head atom (may be typo); if still unknown symbol → error.
	h2, err := expand(head, env, depth+1)
	if err != nil {
		return nil, err
	}
	if s, ok := h2.(string); ok {
		if fn, ok := env[s].(expandFn); ok {
			args := make([]any, 0, len(x)-1)
			for _, a := range x[1:] {
				ea, err := expand(a, env, depth+1)
				if err != nil {
					return nil, err
				}
				args = append(args, ea)
			}
			return applyExpandFn(fn, args, env, depth)
		}
		if isExpandKnownHead(s) {
			out := make([]any, len(x))
			out[0] = s
			for i := 1; i < len(x); i++ {
				ea, err := expand(x[i], env, depth+1)
				if err != nil {
					return nil, err
				}
				out[i] = ea
			}
			return out, nil
		}
		return nil, fmt.Errorf("%w: expand: unknown head %q", ErrExpand, s)
	}
	return nil, fmt.Errorf("%w: expand: unknown head (got %T after expand)", ErrExpand, h2)
}

func expandLet(args []any, env expandEnv, depth int) (any, error) {
	// (let n1 v1 n2 v2 ... body) — at least name, val, body → 3 elements; or just body?
	// Require at least body; zero bindings allowed: (let body)
	if len(args) == 0 {
		return nil, fmt.Errorf("%w: expand: let needs a body", ErrExpand)
	}
	if len(args)%2 == 0 {
		// name val name val — missing body
		return nil, fmt.Errorf("%w: expand: let needs alternating bindings and a body", ErrExpand)
	}
	// Last is body; preceding pairs are bindings.
	e := env.clone()
	for i := 0; i < len(args)-1; i += 2 {
		name, ok := args[i].(string)
		if !ok {
			return nil, fmt.Errorf("%w: expand: let binding name must be a symbol, got %T", ErrExpand, args[i])
		}
		val, err := expand(args[i+1], e, depth+1)
		if err != nil {
			return nil, err
		}
		e[name] = val
	}
	return expand(args[len(args)-1], e, depth+1)
}

func expandProgn(args []any, env expandEnv, depth int) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%w: expand: empty progn", ErrExpand)
	}
	e := env.clone()
	var last any
	for i, f := range args {
		if list, ok := f.([]any); ok && len(list) > 0 {
			if h, ok := list[0].(string); ok && h == "def" {
				name, fn, err := parseDefForm(list[1:])
				if err != nil {
					return nil, err
				}
				e[name] = fn
				if i == len(args)-1 {
					// progn ending in def: result is the function value
					last = fn
				}
				continue
			}
		}
		var err error
		last, err = expand(f, e, depth+1)
		if err != nil {
			return nil, err
		}
	}
	if last == nil {
		return nil, fmt.Errorf("%w: expand: progn produced no value", ErrExpand)
	}
	return last, nil
}

func parseExpandFn(args []any) (expandFn, error) {
	// (fn (p1 p2 …) body) | (fn (p1 p2 …) "docstring" body)
	params, body, err := parseParamsDocBody(args, "fn")
	if err != nil {
		return expandFn{}, err
	}
	return expandFn{params: params, body: body}, nil
}

func parseDefForm(args []any) (name string, fn expandFn, err error) {
	// (def name (params) body) | (def name (params) "docstring" body)
	if len(args) < 3 {
		return "", expandFn{}, fmt.Errorf("%w: expand: def needs name, (params), and body", ErrExpand)
	}
	n, ok := args[0].(string)
	if !ok {
		return "", expandFn{}, fmt.Errorf("%w: expand: def name must be a symbol", ErrExpand)
	}
	params, body, err := parseParamsDocBody(args[1:], "def")
	if err != nil {
		return "", expandFn{}, err
	}
	return n, expandFn{params: params, body: body}, nil
}

// parseParamsDocBody parses (params) body or (params) "docstring" body.
// The docstring is documentation only and is discarded at expand time.
func parseParamsDocBody(args []any, what string) (params []string, body any, err error) {
	if len(args) < 2 {
		return nil, nil, fmt.Errorf("%w: expand: %s needs (params) and body", ErrExpand, what)
	}
	params, err = parseParamList(args[0])
	if err != nil {
		return nil, nil, err
	}
	switch len(args) {
	case 2:
		return params, cloneForm(args[1]), nil
	case 3:
		if _, ok := args[1].(string); !ok {
			return nil, nil, fmt.Errorf("%w: expand: %s optional docstring must be a string", ErrExpand, what)
		}
		return params, cloneForm(args[2]), nil
	default:
		return nil, nil, fmt.Errorf("%w: expand: %s wants (params) [docstring] body", ErrExpand, what)
	}
}

func parseParamList(v any) ([]string, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: expand: params must be a list", ErrExpand)
	}
	out := make([]string, 0, len(list))
	for _, p := range list {
		s, ok := p.(string)
		if !ok {
			return nil, fmt.Errorf("%w: expand: param must be a symbol, got %T", ErrExpand, p)
		}
		out = append(out, s)
	}
	return out, nil
}

func applyExpandFn(fn expandFn, args []any, env expandEnv, depth int) (any, error) {
	if len(args) != len(fn.params) {
		return nil, fmt.Errorf("%w: expand: function wants %d args, got %d", ErrExpand, len(fn.params), len(args))
	}
	e := env.clone()
	for i, p := range fn.params {
		e[p] = args[i]
	}
	return expand(fn.body, e, depth+1)
}

// expandRep implements expand-time (rep N body) only — contiguous unroll.
// (* body), (? body), (+ body) are separate heads; (rep *|?|+ body) is an error.
func expandRep(args []any, env expandEnv, depth int) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("%w: expand: rep wants N and body, got %d args", ErrExpand, len(args))
	}
	// Quant is not env-expanded (must stay a number symbol).
	n, err := expandRepCount(args[0])
	if err != nil {
		return nil, err
	}
	body, err := expand(args[1], env, depth+1)
	if err != nil {
		return nil, err
	}
	return unrollContiguous(n, body)
}

func expandRepCount(v any) (int, error) {
	switch q := v.(type) {
	case string:
		switch q {
		case "*", "+", "?":
			return 0, fmt.Errorf("%w: expand: (rep %s body) is forbidden; use (%s body)", ErrExpand, q, q)
		default:
			n, err := strconv.Atoi(q)
			if err != nil {
				return 0, fmt.Errorf("%w: expand: rep count %q (want non-negative integer)", ErrExpand, q)
			}
			if n < 0 {
				return 0, fmt.Errorf("%w: expand: rep count %d invalid", ErrExpand, n)
			}
			return n, nil
		}
	case float64:
		if q != float64(int(q)) || q < 0 {
			return 0, fmt.Errorf("%w: expand: rep count must be a non-negative integer, got %v", ErrExpand, q)
		}
		return int(q), nil
	case int:
		if q < 0 {
			return 0, fmt.Errorf("%w: expand: rep count %d invalid", ErrExpand, q)
		}
		return q, nil
	default:
		return 0, fmt.Errorf("%w: expand: rep count want integer, got %T", ErrExpand, v)
	}
}

// unrollContiguous builds a contiguous (seq p p … p) of n copies of body.
func unrollContiguous(n int, body any) (any, error) {
	if n < 0 {
		return nil, fmt.Errorf("%w: expand: rep count %d invalid", ErrExpand, n)
	}
	if n == 0 {
		return []any{"seq"}, nil
	}
	if n == 1 {
		return cloneForm(body), nil
	}
	out := make([]any, 0, n+1)
	out = append(out, "seq")
	for i := 0; i < n; i++ {
		out = append(out, cloneForm(body))
	}
	return out, nil
}

func isProtectedNameSlot(head string, argIndex int) bool {
	// argIndex is 1-based index in full list... we pass i from loop where i>=1
	// capture/unify: (capture name body) → index 1 is name
	switch head {
	case "capture", "unify":
		return argIndex == 1
	default:
		return false
	}
}

func isExpandKnownHead(h string) bool {
	switch h {
	// Expand (should not remain after full expand of known ops' interiors)
	case "let", "fn", "def", "progn", "get":
		return true
	// Matcher / scope / site combinators (kept as data for later compile)
	case "path", "under", "and", "or", "not",
		"lang", "family", "node", "as-language", "as-family", "as-layout", "as-directory-representant", "as-grammar",
		"as-package", "as-atom", "as-use", "as-scope", "as-decl", "as-flow", "as-import",
		"as-atomic", "as-docstring", "as-reexport", "as-default",
		"as-keyword", "as-string", "as-number", "as-comment",
		"as-type", "as-const", "as-ident", "as-op", "as-punct",
		"scope", "this",
		"rewrite", "take", "slot", "rule", "builtin":
		return true
	// Core (rep/?/+/ * handled specially above)
	case "any", "rest", "token", "regex", "equals", "ref", "seq", "alt",
		"group", "assert_not_behind", "capture", "unify":
		return true
	default:
		return false
	}
}

func (e expandEnv) clone() expandEnv {
	if e == nil {
		return expandEnv{}
	}
	out := make(expandEnv, len(e))
	for k, v := range e {
		out[k] = v
	}
	return out
}

func cloneForm(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = cloneForm(x[i])
		}
		return out
	case expandFn:
		return expandFn{params: append([]string(nil), x.params...), body: cloneForm(x.body)}
	default:
		return x
	}
}

// formString returns a debug string for expand errors (not full printer).
func formString(v any) string {
	return fmt.Sprintf("%v", v)
}
