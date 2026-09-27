package pattern

import (
	"fmt"
	"regexp"
)

// NodeToPat converts leftover Node IR into Core Pat.
// Multi flags become rep(q, capture|unify(name, body)).
func NodeToPat(n Node) (Pat, error) {
	return nodeToPat(n)
}

func nodeToPat(n Node) (Pat, error) {
	if n.MultiOptional {
		body := n
		body.MultiOptional = false
		body.Multi = false
		body.MultiPlus = false
		inner, err := nodeToPatNoMulti(body)
		if err != nil {
			return nil, err
		}
		return Rep{LocalOptional: true, Min: 0, Max: 1, Body: inner}, nil
	}
	// Multi on this node: strip multi, convert body, wrap rep outside binder.
	if n.Multi {
		body := n
		body.Multi = false
		body.MultiPlus = false
		body.RepMin = 0
		body.RepMax = 0
		inner, err := nodeToPatNoMulti(body)
		if err != nil {
			return nil, err
		}
		min, max := nodeGapBounds(n)
		return Rep{Min: min, Max: max, Body: inner}, nil
	}
	return nodeToPatNoMulti(n)
}

func nodeToPatNoMulti(n Node) (Pat, error) {
	switch n.Kind {
	case "rest":
		return Rest(), nil
	case "lit":
		p := Pat(Lit{Text: n.Text})
		return wrapBind(n.As, n.Unify, p), nil
	case "token", "type_token":
		p := Pat(Token{Text: n.Text})
		// As "ROOT" is legacy noise on tokens, not a capture.
		if n.As == "ROOT" {
			return p, nil
		}
		return wrapBind(n.As, n.Unify, p), nil
	case "ref":
		p := Pat(Ref{Target: n.Ref})
		return wrapBind(n.As, n.Unify, p), nil
	case "capture":
		// Bare capture/unify node → capture|unify(name, any)
		if n.As == "" || n.As == "_" {
			return Any{}, nil
		}
		return wrapBind(n.As, n.Unify, Any{}), nil
	case "string":
		p := Pat(Regex{
			RE:           n.Regex,
			Invert:       n.Invert,
			CaptureGroup: n.CaptureGroup,
			Equals:       n.Equals,
			FromCapture:  n.FromCapture,
		})
		return wrapBind(n.As, n.Unify, p), nil
	case "group":
		arms := groupArms(n)
		if len(arms) == 0 {
			return nil, fmt.Errorf("%w: pattern: empty group", ErrCompile)
		}
		var body Pat
		if len(arms) == 1 {
			inner, err := nodeToPat(arms[0])
			if err != nil {
				return nil, err
			}
			// Single arm may already be seq from flatten; keep as-is.
			body = Group{Body: ensureSeqOrSelf(inner)}
		} else {
			items := make([]Pat, 0, len(arms))
			for _, a := range arms {
				ip, err := nodeToPat(a)
				if err != nil {
					return nil, err
				}
				items = append(items, ensureSeqOrSelf(ip))
			}
			body = Group{Body: Alt{Items: items}}
		}
		if n.Invert {
			// assert_not_behind; bind name ignored for lookbehind binds (engine discards)
			// Named invert group still has As for parse; Core drops bind on assert.
			return AssertNotBehind{Body: body.(Group).Body}, nil
		}
		// group(body) then optional capture/unify
		g := body
		return wrapBind(n.As, n.Unify, g), nil
	case "seq":
		items := make([]Pat, 0, len(n.Args))
		for _, a := range n.Args {
			ip, err := nodeToPat(a)
			if err != nil {
				return nil, err
			}
			items = append(items, ip)
		}
		return Seq{Items: items}, nil
	case "call":
		// Desugar to seq via flattenPattern semantics.
		flat := flattenPattern(n)
		items := make([]Pat, 0, len(flat))
		for _, a := range flat {
			ip, err := nodeToPat(a)
			if err != nil {
				return nil, err
			}
			items = append(items, ip)
		}
		return Seq{Items: items}, nil
	case "template":
		return nil, fmt.Errorf("%w: pattern: template is not a match pat", ErrCompile)
	case "":
		return nil, fmt.Errorf("%w: pattern: empty node kind", ErrCompile)
	default:
		return nil, fmt.Errorf("%w: pattern: unknown node kind %q", ErrCompile, n.Kind)
	}
}

func ensureSeqOrSelf(p Pat) Pat {
	return p
}

func wrapBind(name string, unify bool, body Pat) Pat {
	if name == "" || name == "_" || name == "ROOT" {
		return body
	}
	if unify {
		return Unify{Name: name, Body: body}
	}
	return Capture{Name: name, Body: body}
}

// PatToNode converts Core Pat back to legacy Node IR for the current NFA compiler.
// Only forms expressible in today's Node dialect are supported; LocalOptional and
// numeric rep bounds other than * / + error until Phase 2.
func PatToNode(p Pat) (Node, error) {
	return patToNode(p)
}

func patToNode(p Pat) (Node, error) {
	switch x := p.(type) {
	case Any:
		return Node{Kind: "capture"}, nil // unbound any-token
	case Lit:
		return Node{Kind: "lit", Text: x.Text}, nil
	case Token:
		return Node{Kind: "token", Text: x.Text, As: "ROOT"}, nil
	case Regex:
		return Node{
			Kind:         "string",
			Regex:        x.RE,
			Invert:       x.Invert,
			CaptureGroup: x.CaptureGroup,
			Equals:       x.Equals,
			FromCapture:  x.FromCapture,
		}, nil
	case Ref:
		return Node{Kind: "ref", Ref: x.Target}, nil
	case Not:
		// Legacy Node only has invert on regex. Lower unit not(…) the same way
		// compileNot does: lit/token → inverted equals; regex → Invert.
		switch inner := x.Inner.(type) {
		case Regex:
			inner.Invert = true
			return patToNode(inner)
		case Lit:
			return patToNode(Regex{RE: "^" + regexp.QuoteMeta(inner.Text) + "$", Invert: true})
		case Token:
			return patToNode(Regex{RE: "^" + regexp.QuoteMeta(inner.Text) + "$", Invert: true})
		default:
			return Node{}, fmt.Errorf("%w: pattern: not(%T) not representable in legacy Node", ErrCompile, x.Inner)
		}
	case Seq:
		args := make([]Node, 0, len(x.Items))
		for _, it := range x.Items {
			n, err := patToNode(it)
			if err != nil {
				return Node{}, err
			}
			args = append(args, n)
		}
		return Node{Kind: "seq", Args: args}, nil
	case Alt:
		// Legacy alt only exists as group arms.
		args := make([]Node, 0, len(x.Items))
		for _, it := range x.Items {
			n, err := patToNode(it)
			if err != nil {
				return Node{}, err
			}
			args = append(args, n)
		}
		return Node{Kind: "group", As: "_", Args: args}, nil
	case Rep:
		if x.LocalOptional {
			var name string
			var unify bool
			var body Pat
			switch b := x.Body.(type) {
			case Capture:
				name, body = b.Name, b.Body
			case Unify:
				name, unify, body = b.Name, true, b.Body
			default:
				inner, err := patToNode(x.Body)
				if err != nil {
					return Node{}, err
				}
				inner.MultiOptional = true
				inner.RepMin, inner.RepMax = 0, 1
				return inner, nil
			}
			inner, err := patToNode(body)
			if err != nil {
				return Node{}, err
			}
			inner = applyNameUnify(inner, name, unify)
			inner.MultiOptional = true
			inner.RepMin, inner.RepMax = 0, 1
			return inner, nil
		}
		// rest ≡ rep(*, any)
		if isUnboundedStar(x) {
			if _, ok := x.Body.(Any); ok {
				return Node{Kind: "rest", As: "_"}, nil
			}
		}
		var name string
		var unify bool
		var body Pat
		switch b := x.Body.(type) {
		case Capture:
			name, body = b.Name, b.Body
		case Unify:
			name, unify, body = b.Name, true, b.Body
		default:
			inner, err := patToNode(x.Body)
			if err != nil {
				return Node{}, err
			}
			return applyGapRep(inner, x.Min, x.Max), nil
		}
		inner, err := patToNode(body)
		if err != nil {
			return Node{}, err
		}
		inner = applyNameUnify(inner, name, unify)
		return applyGapRep(inner, x.Min, x.Max), nil
	case Group:
		return groupToNode(x.Body, "", false, false)
	case AssertNotBehind:
		n, err := groupToNode(x.Body, "_", false, true)
		if err != nil {
			return Node{}, err
		}
		n.Invert = true
		return n, nil
	case Capture:
		return binderToNode(x.Name, false, x.Body)
	case Unify:
		return binderToNode(x.Name, true, x.Body)
	default:
		return Node{}, fmt.Errorf("%w: pattern: unknown pat %T", ErrCompile, p)
	}
}

func applyNameUnify(n Node, name string, unify bool) Node {
	if name == "" || name == "_" {
		return n
	}
	n.As = name
	n.Unify = unify
	return n
}

func applyGapRep(n Node, min, max int) Node {
	n.Multi = true
	n.MultiOptional = false
	n.RepMin = min
	n.RepMax = max
	n.MultiPlus = min == 1 && max < 0
	return n
}

func binderToNode(name string, unify bool, body Pat) (Node, error) {
	switch body.(type) {
	case Any:
		return Node{Kind: "capture", As: name, Unify: unify}, nil
	default:
		n, err := patToNode(body)
		if err != nil {
			return Node{}, err
		}
		return applyNameUnify(n, name, unify), nil
	}
}

func groupToNode(body Pat, as string, unify, invert bool) (Node, error) {
	n := Node{Kind: "group", As: as, Unify: unify, Invert: invert}
	switch b := body.(type) {
	case Alt:
		args := make([]Node, 0, len(b.Items))
		for _, it := range b.Items {
			an, err := patToNode(it)
			if err != nil {
				return Node{}, err
			}
			args = append(args, an)
		}
		n.Args = args
		if len(args) == 1 {
			a := args[0]
			n.Callee = &a
		}
	default:
		an, err := patToNode(body)
		if err != nil {
			return Node{}, err
		}
		n.Args = []Node{an}
		n.Callee = &an
	}
	if as == "" {
		n.As = "_"
	}
	return n, nil
}
