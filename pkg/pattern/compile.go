package pattern

import (
	"fmt"
	"regexp"
)

// CompilePat builds an NFA from Core Pat (canonical match IR).
func CompilePat(p Pat) (*nfa, error) {
	return compilePat(p)
}

func compilePat(p Pat) (*nfa, error) {
	if err := CheckPat(p); err != nil {
		return nil, err
	}
	c := &nfaCompiler{}
	f, err := c.compileP(p)
	if err != nil {
		return nil, err
	}
	c.n.start = f.start
	c.n.accept = f.accept
	c.n.startPreds = analyzeStartPreds(&c.n, func(s int) bool { return s == c.n.accept })
	return &c.n, nil
}

// compilePattern compiles legacy Node IR via Core Pat (no Pat→Node round-trip).
func compilePattern(pat Node) (*nfa, error) {
	core, err := NodeToPat(pat)
	if err != nil {
		return nil, err
	}
	return compilePat(core)
}

func (c *nfaCompiler) compileP(p Pat) (nfaFrag, error) {
	if p == nil {
		return nfaFrag{}, fmt.Errorf("%w: pattern: nil pat", ErrCompile)
	}
	switch x := p.(type) {
	case Any:
		return c.compileConsume(nfaPred{kind: predCaptureAny}, nil), nil
	case Lit:
		return c.compileConsume(nfaPred{kind: predLit, text: x.Text}, nil), nil
	case Token:
		return c.compileConsume(nfaPred{kind: predToken, text: x.Text}, nil), nil
	case Ref:
		return c.compileConsume(nfaPred{kind: predRef, ref: x.Target}, nil), nil
	case Regex:
		pred, ops, err := regexPredOps(x, "", false)
		if err != nil {
			return nfaFrag{}, err
		}
		return c.compileConsume(pred, ops), nil
	case Not:
		return c.compileNot(x.Inner)
	case Seq:
		return c.compileSeqP(x.Items)
	case Alt:
		return c.compileAltP(x.Items)
	case Rep:
		// rest ≡ rep(*, any) uses the open rest loop (predAny), not gap multi.
		if isUnboundedStar(x) {
			if _, ok := x.Body.(Any); ok {
				return c.compileRest(""), nil
			}
		}
		if x.LocalOptional {
			return c.compileLocalOptionalP(x.Body)
		}
		return c.compileGapRepP(x.Min, x.Max, x.Body)
	case Group:
		return c.compileGroupP(x.Body, "", false)
	case AssertNotBehind:
		return c.compileAssertNotBehindP(x.Body)
	case Capture:
		return c.compileBind(false, x.Name, x.Body)
	case Unify:
		return c.compileBind(true, x.Name, x.Body)
	default:
		return nfaFrag{}, fmt.Errorf("%w: pattern: unknown pat %T", ErrCompile, p)
	}
}

func (c *nfaCompiler) compileSeqP(items []Pat) (nfaFrag, error) {
	if len(items) == 0 {
		s := c.st()
		return nfaFrag{s, s}, nil
	}
	frags := make([]nfaFrag, 0, len(items))
	for _, it := range items {
		f, err := c.compileP(it)
		if err != nil {
			return nfaFrag{}, err
		}
		frags = append(frags, f)
	}
	for i := 0; i < len(frags)-1; i++ {
		c.eps(frags[i].accept, frags[i+1].start)
	}
	return nfaFrag{frags[0].start, frags[len(frags)-1].accept}, nil
}

func (c *nfaCompiler) compileAltP(items []Pat) (nfaFrag, error) {
	if len(items) == 0 {
		return nfaFrag{}, fmt.Errorf("%w: pattern: empty alt", ErrCompile)
	}
	if len(items) == 1 {
		return c.compileP(items[0])
	}
	s, join := c.st(), c.st()
	for _, it := range items {
		f, err := c.compileP(it)
		if err != nil {
			return nfaFrag{}, err
		}
		c.eps(s, f.start)
		c.eps(f.accept, join)
	}
	return nfaFrag{s, join}, nil
}

func (c *nfaCompiler) compileNot(inner Pat) (nfaFrag, error) {
	if !isUnitPred(inner) {
		return nfaFrag{}, fmt.Errorf("%w: pattern: not requires a unit predicate", ErrCompile)
	}
	// Invert is expressed on Regex (or Not wrapping unit).
	switch u := inner.(type) {
	case Regex:
		u.Invert = true
		pred, ops, err := regexPredOps(u, "", false)
		if err != nil {
			return nfaFrag{}, err
		}
		return c.compileConsume(pred, ops), nil
	case Lit:
		// token text must not equal lit — encode as inverted equals on raw token via regex?
		// predEquals is content; for lit use invert regex on full token text is awkward.
		// Use Not only for Regex in practice; lit invert: match any and fail if equal — not a single pred.
		// Compile as regex with quoted literal invert on content map for strings, raw for idents.
		re := regexp.QuoteMeta(u.Text)
		pred, ops, err := regexPredOps(Regex{RE: "^" + re + "$", Invert: true}, "", false)
		if err != nil {
			return nfaFrag{}, err
		}
		// regex runs on unquoted content for strings — for idents content=text. OK for idents.
		// For punctuation lit, content is the lit. OK.
		return c.compileConsume(pred, ops), nil
	case Token:
		re := regexp.QuoteMeta(u.Text)
		pred, ops, err := regexPredOps(Regex{RE: "^" + re + "$", Invert: true}, "", false)
		if err != nil {
			return nfaFrag{}, err
		}
		return c.compileConsume(pred, ops), nil
	case Ref:
		// no invert-ref pred; reject for now
		return nfaFrag{}, fmt.Errorf("%w: pattern: not(ref) not supported", ErrCompile)
	case Any:
		return nfaFrag{}, fmt.Errorf("%w: pattern: not(any) is empty", ErrCompile)
	case Not:
		// double not
		return c.compileP(u.Inner)
	default:
		return nfaFrag{}, fmt.Errorf("%w: pattern: not(%T) not supported", ErrCompile, inner)
	}
}

func (c *nfaCompiler) compileBind(unify bool, name string, body Pat) (nfaFrag, error) {
	if name == "" || name == "_" || name == "ROOT" {
		return c.compileP(body)
	}
	switch b := body.(type) {
	case Any:
		return c.compileConsume(nfaPred{kind: predCaptureAny}, bindOps(name, unify)), nil
	case Lit:
		return c.compileConsume(nfaPred{kind: predLit, text: b.Text}, bindOps(name, unify)), nil
	case Token:
		return c.compileConsume(nfaPred{kind: predToken, text: b.Text}, bindOps(name, unify)), nil
	case Ref:
		return c.compileConsume(nfaPred{kind: predRef, ref: b.Target}, bindOpsRef(name, unify)), nil
	case Regex:
		pred, ops, err := regexPredOps(b, name, unify)
		if err != nil {
			return nfaFrag{}, err
		}
		return c.compileConsume(pred, ops), nil
	case Not:
		// e.g. capture name (not (regex ...))
		switch u := b.Inner.(type) {
		case Regex:
			u.Invert = true
			pred, ops, err := regexPredOps(u, name, unify)
			if err != nil {
				return nfaFrag{}, err
			}
			return c.compileConsume(pred, ops), nil
		default:
			return nfaFrag{}, fmt.Errorf("%w: pattern: bind not(%T) not supported", ErrCompile, b.Inner)
		}
	case Group:
		return c.compileGroupP(b.Body, name, unify)
	case Seq, Alt:
		// multi-token capture uses covering-node geometry (group)
		return c.compileGroupP(body, name, unify)
	default:
		return nfaFrag{}, fmt.Errorf("%w: pattern: capture/unify around %T not supported", ErrCompile, body)
	}
}

func regexPredOps(x Regex, name string, unify bool) (nfaPred, []capOp, error) {
	p := nfaPred{captureGroup: x.CaptureGroup, invert: x.Invert}
	if x.RE != "" {
		re, err := regexp.Compile(x.RE)
		if err != nil {
			return p, nil, fmt.Errorf("pattern regex: %w", err)
		}
		p.kind, p.regex = predRegex, re
		if x.Invert {
			p.captureGroup = 0
		}
	} else if x.Equals != "" {
		p.kind, p.equals = predEquals, x.Equals
	} else {
		p.kind = predCaptureAny
	}
	if name == "" || name == "_" || name == "ROOT" {
		return p, nil, nil
	}
	if p.kind == predRegex {
		cg := p.captureGroup
		if unify {
			return p, []capOp{{kind: capBindRegex, name: name, captureGroup: cg, unify: true}}, nil
		}
		return p, []capOp{{kind: capAppendRegex, name: name, captureGroup: cg}}, nil
	}
	return p, bindOps(name, unify), nil
}

func (c *nfaCompiler) compileGroupP(body Pat, name string, unify bool) (nfaFrag, error) {
	var arms []Pat
	switch b := body.(type) {
	case Alt:
		arms = b.Items
	default:
		arms = []Pat{body}
	}
	if len(arms) == 0 {
		s := c.st()
		return nfaFrag{s, s}, nil
	}
	compiled := make([]nfaFrag, 0, len(arms))
	for _, arm := range arms {
		f, err := c.compileP(arm)
		if err != nil {
			return nfaFrag{}, err
		}
		compiled = append(compiled, f)
	}
	if name == "" || name == "_" {
		if len(compiled) == 1 {
			return compiled[0], nil
		}
		s, join := c.st(), c.st()
		for _, af := range compiled {
			c.eps(s, af.start)
			c.eps(af.accept, join)
		}
		return nfaFrag{s, join}, nil
	}
	s, mid, join, a := c.st(), c.st(), c.st(), c.st()
	closeK := capClose
	if !unify {
		closeK = capAppend
	}
	c.eps(s, mid, capOp{kind: capOpen, name: name})
	for _, af := range compiled {
		c.eps(mid, af.start)
		c.eps(af.accept, join)
	}
	c.eps(join, a, capOp{kind: closeK, name: name, unify: unify})
	return nfaFrag{s, a}, nil
}

func (c *nfaCompiler) compileAssertNotBehindP(body Pat) (nfaFrag, error) {
	sub, err := compilePositivePatNFA(body)
	if err != nil {
		return nfaFrag{}, err
	}
	s, a := c.st(), c.st()
	c.n.states[s].eps = append(c.n.states[s].eps, epsEdge{
		to:            a,
		negLookbehind: sub,
	})
	return nfaFrag{s, a}, nil
}

func compilePositivePatNFA(p Pat) (*nfa, error) {
	c := &nfaCompiler{}
	// Structure only: strip binder names by compiling a bind-free copy where possible.
	// Nested binds in lookbehind are discarded at runtime (sub-NFA); OK to compile as-is.
	f, err := c.compileP(p)
	if err != nil {
		return nil, err
	}
	c.n.start = f.start
	c.n.accept = f.accept
	attachLookbehindStarNot(&c.n, p)
	return &c.n, nil
}

func attachLookbehindStarNot(n *nfa, body Pat) {
	if n == nil {
		return
	}
	first, stop, ok := starNotLookbehindFromPat(body)
	if !ok {
		return
	}
	n.lookbehindStarNot = true
	n.lookbehindStarNotFirst = first
	n.lookbehindStarNotStop = stop
}

func unwrapPatGroup(p Pat) Pat {
	for {
		g, ok := p.(Group)
		if !ok {
			return p
		}
		p = g.Body
	}
}

func atomicLookbehindPred(p Pat) (nfaPred, bool) {
	switch x := unwrapPatGroup(p).(type) {
	case Lit:
		return nfaPred{kind: predLit, text: x.Text}, true
	case Token:
		return nfaPred{kind: predToken, text: x.Text}, true
	case Capture:
		return atomicLookbehindPred(x.Body)
	case Unify:
		return atomicLookbehindPred(x.Body)
	default:
		return nfaPred{}, false
	}
}

// starNotLookbehindFromPat is (seq FIRST (* (not STOP))) with unit FIRST/STOP.
func starNotLookbehindFromPat(p Pat) (first, stop nfaPred, ok bool) {
	p = unwrapPatGroup(p)
	seq, isSeq := p.(Seq)
	if !isSeq || len(seq.Items) != 2 {
		return nfaPred{}, nfaPred{}, false
	}
	first, ok = atomicLookbehindPred(seq.Items[0])
	if !ok {
		return nfaPred{}, nfaPred{}, false
	}
	rep, isRep := unwrapPatGroup(seq.Items[1]).(Rep)
	if !isRep || !isUnboundedStar(rep) {
		return nfaPred{}, nfaPred{}, false
	}
	stop, ok = starBodyStopPred(rep.Body)
	return first, stop, ok
}

// starBodyStopPred is STOP in (* (not STOP)). After PatToNode, not(lit)
// becomes an inverted regex.
func starBodyStopPred(p Pat) (nfaPred, bool) {
	switch x := unwrapPatGroup(p).(type) {
	case Not:
		return atomicLookbehindPred(x.Inner)
	case Regex:
		if !x.Invert {
			return nfaPred{}, false
		}
		x.Invert = false
		pred, _, err := regexPredOps(x, "", false)
		if err != nil {
			return nfaPred{}, false
		}
		return pred, true
	default:
		return nfaPred{}, false
	}
}

func (c *nfaCompiler) compileLocalOptionalP(body Pat) (nfaFrag, error) {
	inner, err := c.compileP(body)
	if err != nil {
		return nfaFrag{}, err
	}
	s, a := c.st(), c.st()
	c.eps(s, inner.start)
	c.eps(inner.accept, a)
	c.eps(s, a)
	return nfaFrag{s, a}, nil
}

func (c *nfaCompiler) compileGapRepP(min, max int, body Pat) (nfaFrag, error) {
	if max >= 0 && min > max {
		return nfaFrag{}, fmt.Errorf("%w: pattern: rep min %d > max %d", ErrCompile, min, max)
	}
	if min < 0 {
		return nfaFrag{}, fmt.Errorf("%w: pattern: rep min %d invalid", ErrCompile, min)
	}
	if max < 0 {
		return c.compileGapRepUnboundedP(body, min)
	}
	return c.compileGapRepBoundedP(body, min, max)
}

func (c *nfaCompiler) compileGapRepUnboundedP(body Pat, min int) (nfaFrag, error) {
	s, mid, a := c.st(), c.st(), c.st()
	cur := s
	for i := 0; i < min; i++ {
		frag, err := c.compileP(body)
		if err != nil {
			return nfaFrag{}, err
		}
		if i > 0 {
			gap := c.st()
			c.eps(cur, gap)
			c.edge(gap, nfaEdge{to: gap, pred: nfaPred{kind: predAnyExceptFunc}})
			c.eps(gap, frag.start)
		} else {
			c.eps(cur, frag.start)
		}
		cur = frag.accept
	}
	c.eps(cur, mid)
	if min == 0 {
		c.eps(s, mid)
		c.eps(mid, a)
	}
	c.eps(mid, a)
	c.edge(mid, nfaEdge{to: mid, pred: nfaPred{kind: predAnyExceptFunc}})
	frag, err := c.compileP(body)
	if err != nil {
		return nfaFrag{}, err
	}
	c.eps(mid, frag.start)
	c.eps(frag.accept, mid)
	return nfaFrag{s, a}, nil
}

func (c *nfaCompiler) compileGapRepBoundedP(body Pat, min, max int) (nfaFrag, error) {
	count := make([]int, max+1)
	for i := range count {
		count[i] = c.st()
	}
	a := c.st()
	s := count[0]
	if min == 0 {
		c.eps(count[0], a)
	}
	for i := 0; i <= max; i++ {
		c.edge(count[i], nfaEdge{to: count[i], pred: nfaPred{kind: predAnyExceptFunc}})
		if i >= min {
			c.eps(count[i], a)
		}
		if i == max {
			continue
		}
		frag, err := c.compileP(body)
		if err != nil {
			return nfaFrag{}, err
		}
		c.eps(count[i], frag.start)
		c.eps(frag.accept, count[i+1])
	}
	return nfaFrag{s, a}, nil
}

// looksLikeCallPat reports whether a covering call-ish AST node should widen the match span.
func looksLikeCallPat(p Pat) bool {
	var walk func(Pat) bool
	walk = func(p Pat) bool {
		switch x := p.(type) {
		case Lit:
			return x.Text == "("
		case Seq:
			for _, it := range x.Items {
				if walk(it) {
					return true
				}
			}
		case Alt:
			for _, it := range x.Items {
				if walk(it) {
					return true
				}
			}
		case Capture:
			return walk(x.Body)
		case Unify:
			return walk(x.Body)
		case Rep:
			return walk(x.Body)
		case Group:
			return walk(x.Body)
		case AssertNotBehind:
			return walk(x.Body)
		case Not:
			return walk(x.Inner)
		}
		return false
	}
	return walk(p)
}
