package pattern

// NewExpandEnv returns an empty expand environment for script loading.
func NewExpandEnv() expandEnv {
	return expandEnv{}
}

// Set binds name → value in the expand env (used for top-level def).
func (e expandEnv) Set(name string, value any) {
	if e == nil {
		return
	}
	e[name] = value
}

// CloneExpandEnv returns a copy of env (for prelude loaders).
func CloneExpandEnv(env expandEnv) expandEnv {
	return env.clone()
}

// ParseDef parses (def …) args after the head: name (params) [doc] body.
func ParseDef(args []any) (name string, fn any, err error) {
	n, f, err := parseDefForm(args)
	if err != nil {
		return "", nil, err
	}
	return n, f, nil
}

// MatcherFromExpanded builds a Matcher from post-Expand data.
func MatcherFromExpanded(v any) (Matcher, error) {
	return matcherFromExpanded(v)
}

// RefEmitText is the public form of product-ref selector emit for rewrite templates/emit trees.
func RefEmitText(ref string) (string, error) {
	return refEmitText(ref)
}
