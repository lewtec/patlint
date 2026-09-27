package pattern

import (
	"fmt"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
	"regexp"
)

// NFA over pattern AST. Alphabet = source tokens. ε for concat / empty rest /
// star. Capture open/close/append are side effects on edges.

type nfa struct {
	start, accept int
	states        []nfaState
	// startPreds, when non-empty, is the set of first-token predicates reachable
	// from start by ε only. matchCompiled / multi skip positions that match none.
	// Nil means "no filter" (any/rest/lookbehind/empty-match).
	startPreds []nfaPred
	// maxTokenConsume is the longest start→accept token count. -1 is unbounded
	// (consuming cycle can still reach accept). Set on first tokenConsumeBound.
	maxTokenConsume int
	tokenBoundKnown bool
	// lookbehindStarNot is (seq FIRST (* (not STOP))) — Java visibility.
	// Gap-repeat after FIRST can consume any token except the dead pair
	// (func and STOP); match ending at endPos iff FIRST occurs before it.
	lookbehindStarNot      bool
	lookbehindStarNotFirst nfaPred
	lookbehindStarNotStop  nfaPred
}

type nfaState struct {
	eps   []epsEdge
	edges []nfaEdge
}

type epsEdge struct {
	to  int
	ops []capOp
	// negLookbehind, when set: take this ε only if the sub-NFA does **not**
	// match any token span ending at the thread's current pos (zero-width
	// negative lookbehind for (assert_not_behind …)).
	negLookbehind *nfa
}

type nfaEdge struct {
	to   int
	pred nfaPred
	ops  []capOp
}

type nfaPred struct {
	kind         predKind
	text         string
	ref          string
	regex        *regexp.Regexp
	equals       string
	captureGroup int
	invert       bool // predRegex: succeed when regex does not match
}

type predKind int

const (
	predAny           predKind = iota
	predAnyExceptFunc          // like any, but not the "func" keyword (multi stays in one function)
	predLit
	predToken
	predRef
	predRegex
	predEquals
	predCaptureAny
)

type capOpKind int

const (
	capOpen capOpKind = iota
	capClose
	capAppend
	capBindToken
	capBindRef
	capBindRegex
	capAppendToken
	capAppendRef
	capAppendRegex
)

type capOp struct {
	kind         capOpKind
	name         string
	captureGroup int
	unify        bool // % discipline (ignored when multi-body forces append kinds)
}

type nfaFrag struct{ start, accept int }

type nfaCompiler struct {
	n nfa
}

func (c *nfaCompiler) st() int {
	c.n.states = append(c.n.states, nfaState{})
	return len(c.n.states) - 1
}

func (c *nfaCompiler) eps(from, to int, ops ...capOp) {
	c.n.states[from].eps = append(c.n.states[from].eps, epsEdge{to: to, ops: ops})
}

func (c *nfaCompiler) edge(from int, e nfaEdge) {
	c.n.states[from].edges = append(c.n.states[from].edges, e)
}

// compileLegacyNode builds an NFA from legacy Node IR by converting to Core Pat.
// Prefer CompilePat / compilePat for new code.
func compileLegacyNode(pat Node) (*nfa, error) {
	return compilePattern(pat)
}

func (c *nfaCompiler) compileSeq(seq []Node) (nfaFrag, error) {
	items := make([]Pat, 0, len(seq))
	for _, step := range seq {
		p, err := NodeToPat(step)
		if err != nil {
			return nfaFrag{}, err
		}
		items = append(items, p)
	}
	return c.compileSeqP(items)
}

func (c *nfaCompiler) compileNode(n Node) (nfaFrag, error) {
	if n.MultiOptional {
		body := n
		body.MultiOptional = false
		return c.compileLocalOptional(body)
	}
	if n.Multi {
		return c.compileGapRep(n)
	}
	switch n.Kind {
	case "rest":
		return c.compileRest(n.As), nil
	case "lit":
		return c.compileConsume(nfaPred{kind: predLit, text: n.Text}, bindOps(n.As, n.Unify)), nil
	case "token", "type_token":
		return c.compileConsume(nfaPred{kind: predToken, text: n.Text}, bindOps(n.As, n.Unify)), nil
	case "ref":
		return c.compileConsume(nfaPred{kind: predRef, ref: n.Ref}, bindOpsRef(n.As, n.Unify)), nil
	case "capture":
		return c.compileConsume(nfaPred{kind: predCaptureAny}, bindOps(n.As, n.Unify)), nil
	case "string":
		p, ops, err := stringPredOps(n)
		if err != nil {
			return nfaFrag{}, err
		}
		return c.compileConsume(p, ops), nil
	case "group":
		if n.Invert {
			return c.compileInvertGroup(n)
		}
		return c.compileGroup(n)
	case "seq":
		return c.compileSeq(n.Args)
	case "call":
		return c.compileSeq(flattenPattern(n))
	default:
		s := c.st()
		return nfaFrag{s, s}, nil
	}
}

// bindOps builds token bind ops. $ appends; % unifies (including under gap multi).
func bindOps(as string, unify bool) []capOp {
	if as == "" || as == "_" || as == "ROOT" {
		return nil
	}
	if unify {
		return []capOp{{kind: capBindToken, name: as, unify: true}}
	}
	return []capOp{{kind: capAppendToken, name: as, unify: false}}
}

func bindOpsRef(as string, unify bool) []capOp {
	if as == "" || as == "_" || as == "ROOT" {
		return nil
	}
	if unify {
		return []capOp{{kind: capBindRef, name: as, unify: true}}
	}
	return []capOp{{kind: capAppendRef, name: as, unify: false}}
}

func stringPredOps(n Node) (nfaPred, []capOp, error) {
	p := nfaPred{captureGroup: n.CaptureGroup, invert: n.Invert}
	if n.Regex != "" {
		re, err := regexp.Compile(n.Regex)
		if err != nil {
			return p, nil, fmt.Errorf("pattern regex: %w", err)
		}
		p.kind, p.regex = predRegex, re
		// Invert never rebinds from submatches (there are none on a successful miss).
		if n.Invert {
			p.captureGroup = 0
		}
	} else if n.Equals != "" {
		p.kind, p.equals = predEquals, n.Equals
	} else {
		p.kind = predCaptureAny
	}
	as := n.As
	if as == "" || as == "_" || as == "ROOT" {
		return p, nil, nil
	}
	if p.kind == predRegex {
		cg := p.captureGroup
		if n.Unify {
			return p, []capOp{{kind: capBindRegex, name: as, captureGroup: cg, unify: true}}, nil
		}
		return p, []capOp{{kind: capAppendRegex, name: as, captureGroup: cg}}, nil
	}
	return p, bindOps(as, n.Unify), nil
}

func (c *nfaCompiler) compileConsume(p nfaPred, ops []capOp) nfaFrag {
	s, a := c.st(), c.st()
	c.edge(s, nfaEdge{to: a, pred: p, ops: ops})
	return nfaFrag{s, a}
}

func (c *nfaCompiler) compileRest(as string) nfaFrag {
	// loop with optional empty: s -ε-> mid -any*-> mid -ε-> a ; s -ε-> a
	s, mid, a := c.st(), c.st(), c.st()
	c.eps(s, mid)
	c.eps(s, a)
	if as != "" && as != "_" {
		// open sticky on first any; close when exiting mid -> a
		c.edge(mid, nfaEdge{
			to:   mid,
			pred: nfaPred{kind: predAny},
			ops:  []capOp{{kind: capOpen, name: as}},
		})
		c.eps(mid, a, capOp{kind: capClose, name: as})
	} else {
		c.edge(mid, nfaEdge{to: mid, pred: nfaPred{kind: predAny}})
		c.eps(mid, a)
	}
	return nfaFrag{s, a}
}

func (c *nfaCompiler) compileGroup(n Node) (nfaFrag, error) {
	arms := groupArms(n)
	if len(arms) == 0 {
		s := c.st()
		return nfaFrag{s, s}, nil
	}
	// Compile each arm; ε-split from mid, ε-merge to join.
	type armFrag struct{ start, accept int }
	compiled := make([]armFrag, 0, len(arms))
	for _, arm := range arms {
		inner := flattenPattern(arm)
		body, err := c.compileSeq(inner)
		if err != nil {
			return nfaFrag{}, err
		}
		compiled = append(compiled, armFrag{body.start, body.accept})
	}
	// Non-capturing $_ or empty name: alt without open/close.
	if n.As == "" || n.As == "_" {
		if len(compiled) == 1 {
			return nfaFrag{compiled[0].start, compiled[0].accept}, nil
		}
		s, join := c.st(), c.st()
		for _, af := range compiled {
			c.eps(s, af.start)
			c.eps(af.accept, join)
		}
		return nfaFrag{s, join}, nil
	}
	s, mid, join, a := c.st(), c.st(), c.st(), c.st()
	// $ → append on close; % → unify on close.
	closeK := capClose
	unify := n.Unify
	if !n.Unify {
		closeK = capAppend
	}
	c.eps(s, mid, capOp{kind: capOpen, name: n.As})
	for _, af := range compiled {
		c.eps(mid, af.start)
		c.eps(af.accept, join)
	}
	c.eps(join, a, capOp{kind: closeK, name: n.As, unify: unify})
	return nfaFrag{s, a}, nil
}

// compileInvertGroup builds a zero-width negative lookbehind assertion.
// At the current token index, the positive group must not match any span
// ending there; on success no tokens are consumed and inner captures are ignored.
func (c *nfaCompiler) compileInvertGroup(n Node) (nfaFrag, error) {
	if n.Multi || n.MultiOptional {
		return nfaFrag{}, fmt.Errorf("%w: pattern: invert group quantifier is not supported", ErrCompile)
	}
	sub, err := compilePositiveGroupNFA(n)
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

// compilePositiveGroupNFA compiles the arms of a group as a standalone NFA
// (Invert cleared; no open/close captures — lookbehind only needs accept/reject).
func compilePositiveGroupNFA(n Node) (*nfa, error) {
	pos := n
	pos.Invert = false
	pos.As = "_" // structure only; lookbehind discards binds
	pos.Unify = false
	pos.Multi = false
	pos.MultiPlus = false
	pos.MultiOptional = false
	c := &nfaCompiler{}
	f, err := c.compileGroup(pos)
	if err != nil {
		return nil, err
	}
	c.n.start = f.start
	c.n.accept = f.accept
	return &c.n, nil
}

// compileLocalOptional is rep(?): match body once at the cursor or skip (ε).
// Delegates to Core Pat path (single implementation).
func (c *nfaCompiler) compileLocalOptional(body Node) (nfaFrag, error) {
	p, err := NodeToPat(body)
	if err != nil {
		return nfaFrag{}, err
	}
	return c.compileLocalOptionalP(p)
}

// nodeGapBounds returns min/max for gap-repeat. max < 0 means unbounded.
func nodeGapBounds(n Node) (min, max int) {
	if n.RepMax < 0 || n.RepMin > 0 || n.RepMax > 0 {
		return n.RepMin, n.RepMax
	}
	if n.MultiPlus {
		return 1, -1
	}
	return 0, -1
}

// compileGapRep implements gap multi (* / + / numeric bounds) via Core Pat path.
func (c *nfaCompiler) compileGapRep(n Node) (nfaFrag, error) {
	min, max := nodeGapBounds(n)
	bodyNode := n
	bodyNode.Multi = false
	bodyNode.MultiPlus = false
	bodyNode.MultiOptional = false
	bodyNode.RepMin = 0
	bodyNode.RepMax = 0
	p, err := NodeToPat(bodyNode)
	if err != nil {
		return nfaFrag{}, err
	}
	return c.compileGapRepP(min, max, p)
}

// --- runtime -----------------------------------------------------------------

type nfaThread struct {
	state int
	pos   int // next token index
	open  map[string]int
	caps  map[string]CapSpan
}

// cloneThread deep-copies open/caps. Empty maps stay nil (no alloc) so pure
// lit/ref steps can share maps across forks until a capture write.
func cloneThread(t nfaThread) nfaThread {
	o := nfaThread{state: t.state, pos: t.pos}
	if n := len(t.open); n > 0 {
		o.open = make(map[string]int, n)
		for k, v := range t.open {
			o.open[k] = v
		}
	}
	if n := len(t.caps); n > 0 {
		o.caps = make(map[string]CapSpan, n)
		for k, v := range t.caps {
			o.caps[k] = cloneCapSpan(v)
		}
	}
	return o
}

// stepThread moves state/pos. When own is true, open/caps are deep-copied so
// later writes do not alias sibling threads. When own is false, maps are shared
// (safe only when no subsequent write on this thread without another clone).
func stepThread(cur nfaThread, state, pos int, own bool) nfaThread {
	if own {
		nt := cloneThread(cur)
		nt.state = state
		nt.pos = pos
		return nt
	}
	return nfaThread{state: state, pos: pos, open: cur.open, caps: cur.caps}
}

func ensureOpen(t *nfaThread) {
	if t.open == nil {
		t.open = make(map[string]int)
	}
}

func ensureCaps(t *nfaThread) {
	if t.caps == nil {
		t.caps = make(map[string]CapSpan)
	}
}

// applyOps returns ErrUnifyMismatch (or other bind errors) when a % capture fails.
// Callers drop the thread on non-nil error. Thread maps may be nil until first write.
func applyOps(t *nfaThread, ops []capOp, tokens []tok, source []byte, lastTok int) error {
	// lastTok = index of token just consumed, or -1 for pure ε
	for _, op := range ops {
		if op.name == "" {
			continue
		}
		switch op.kind {
		case capOpen:
			ensureOpen(t)
			if _, ok := t.open[op.name]; !ok {
				if lastTok >= 0 {
					t.open[op.name] = lastTok
				} else {
					t.open[op.name] = t.pos // next token
				}
			}
		case capClose, capAppend:
			start, ok := t.open[op.name]
			if t.open != nil {
				delete(t.open, op.name)
			}
			if !ok {
				continue
			}
			end := t.pos - 1
			if end < start {
				end = start
			}
			if start < 0 || start >= len(tokens) || end < 0 || end >= len(tokens) {
				continue
			}
			sp := ingestutil.Span{
				StartByte: tokens[start].StartByte,
				EndByte:   tokens[end].EndByte,
			}
			// capClose with unify → UnifySpan; capAppend / non-unify close → AppendSpan
			unify := op.kind == capClose && op.unify
			ensureCaps(t)
			if err := bindInto(t.caps, op.name, sp, source, unify); err != nil {
				return err
			}
		case capBindToken, capAppendToken:
			if lastTok < 0 || lastTok >= len(tokens) {
				continue
			}
			unify := op.kind == capBindToken && op.unify
			ensureCaps(t)
			if err := bindInto(t.caps, op.name, tokens[lastTok].Span, source, unify); err != nil {
				return err
			}
		case capBindRef, capAppendRef:
			if lastTok < 0 || lastTok >= len(tokens) {
				continue
			}
			sp := selectorSpan(tokens, lastTok, source)
			unify := op.kind == capBindRef && op.unify
			ensureCaps(t)
			if err := bindInto(t.caps, op.name, sp, source, unify); err != nil {
				return err
			}
		case capBindRegex, capAppendRegex:
			if lastTok < 0 || lastTok >= len(tokens) {
				continue
			}
			sp := bindRegexSpan(tokens[lastTok], source, op.captureGroup)
			unify := op.kind == capBindRegex && op.unify
			ensureCaps(t)
			if err := bindInto(t.caps, op.name, sp, source, unify); err != nil {
				return err
			}
		}
	}
	return nil
}

func bindRegexSpan(t tok, source []byte, captureGroup int) ingestutil.Span {
	// Full-token bind when submatch mapping is not available in this helper.
	_, _ = source, captureGroup
	return t.Span
}

func predMatch(p nfaPred, tokens []tok, pos int, source []byte) (ok bool, named map[string]ingestutil.Span) {
	if pos < 0 || pos >= len(tokens) {
		return false, nil
	}
	t := tokens[pos]
	switch p.kind {
	case predAny, predCaptureAny:
		return true, nil
	case predAnyExceptFunc:
		return !t.Eq(source, "func"), nil
	case predLit, predToken:
		return t.Eq(source, p.text), nil
	case predRef:
		return t.target == p.ref, nil
	case predEquals:
		return tokenContent(source, t) == p.equals, nil
	case predRegex:
		// Fast path: no submatch spans needed (invert or plain match without groups).
		needMap := p.captureGroup > 0 || (p.regex != nil && regexHasNamedGroups(p.regex))
		if !needMap {
			content := tokenContent(source, t)
			if p.equals != "" && content != p.equals {
				return false, nil
			}
			if p.regex == nil {
				return true, nil
			}
			ok := p.regex.MatchString(content)
			if p.invert {
				return !ok, nil
			}
			return ok, nil
		}
		content, srcOf, closeOff, quoted := tokenContentMap(source, t)
		_ = quoted
		if p.equals != "" && content != p.equals {
			return false, nil
		}
		if p.regex == nil {
			return true, nil
		}
		idx := p.regex.FindStringSubmatchIndex(content)
		if p.invert {
			// Succeed when the pattern does not match; bind full token only (no submatches).
			return idx == nil, nil
		}
		if idx == nil {
			return false, nil
		}
		named = map[string]ingestutil.Span{}
		names := p.regex.SubexpNames()
		for i, name := range names {
			if i == 0 || name == "" || 2*i+1 >= len(idx) || idx[2*i] < 0 {
				continue
			}
			sp, ok := contentSpanToSource(t.StartByte, srcOf, closeOff, idx[2*i], idx[2*i+1])
			if ok {
				named[name] = sp
			}
		}
		if p.captureGroup > 0 && 2*p.captureGroup+1 < len(idx) && idx[2*p.captureGroup] >= 0 {
			if sp, ok := contentSpanToSource(t.StartByte, srcOf, closeOff, idx[2*p.captureGroup], idx[2*p.captureGroup+1]); ok {
				named["\x00group"] = sp // internal
			}
		}
		return true, named
	default:
		return false, nil
	}
}

func regexHasNamedGroups(re *regexp.Regexp) bool {
	if re == nil {
		return false
	}
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" {
			return true
		}
	}
	return false
}

// nfaScratch reuses maps/slices across matchNFA / multi simulations so the
// hot ε-closure path does not allocate a seen map on every queue item.
type nfaScratch struct {
	eps  epsScratch
	look epsScratch // nested lookbehind closures (must not clobber eps.out)
	// best capture score seen for state<<32|pos in the current matchNFA run
	seen  map[uint64]int
	queue []nfaThread
}

// epsScratch is one reusable ε-closure workspace (generation-stamped seen).
type epsScratch struct {
	stack []nfaThread
	out   []nfaThread
	stamp map[uint64]uint32
	gen   uint32
	// accept-run (nfaAcceptsAt): generation-stamped seen + queue
	acceptStamp      map[uint64]uint32
	acceptGeneration uint32
	acceptQueue      []nfaThread
}

func (e *epsScratch) bumpGen() {
	e.gen++
	if e.gen == 0 {
		clear(e.stamp)
		e.gen = 1
	}
	if e.stamp == nil {
		e.stamp = make(map[uint64]uint32)
	}
}

func (e *epsScratch) bumpAccept() {
	e.acceptGeneration++
	if e.acceptGeneration == 0 {
		clear(e.acceptStamp)
		e.acceptGeneration = 1
	}
	if e.acceptStamp == nil {
		e.acceptStamp = make(map[uint64]uint32)
	}
	e.acceptQueue = e.acceptQueue[:0]
}

func statePosKey(state, pos int) uint64 {
	return uint64(uint32(state))<<32 | uint64(uint32(pos))
}

// matchNFA runs the NFA from token startIdx. Prefers accepting threads with more
// total capture sites, then shorter end position (stable for multi + rest).
// sc may be nil (allocates a private scratch).
func matchNFA(n *nfa, tokens []tok, startIdx int, source []byte, root *sitter.Node) (Match, int, bool, error) {
	return matchNFAScratch(n, tokens, startIdx, source, root, nil)
}

func matchNFAScratch(n *nfa, tokens []tok, startIdx int, source []byte, root *sitter.Node, sc *nfaScratch) (Match, int, bool, error) {
	if n == nil || len(n.states) == 0 {
		return Match{}, startIdx, false, nil
	}
	if sc == nil {
		sc = &nfaScratch{}
	}
	if sc.seen == nil {
		sc.seen = make(map[uint64]int)
	} else {
		clear(sc.seen)
	}
	sc.queue = sc.queue[:0]

	var best *nfaThread
	bestScore, bestEnd := -1, 1<<30

	// Nil open/caps until first capture write (cloneThread skips empty maps).
	sc.queue = append(sc.queue, nfaThread{state: n.start, pos: startIdx})
	head := 0

	for head < len(sc.queue) {
		t := sc.queue[head]
		head++
		// Compact drained prefix occasionally so the slice does not grow unboundedly.
		if head > 64 && head*2 > len(sc.queue) {
			sc.queue = append(sc.queue[:0], sc.queue[head:]...)
			head = 0
		}

		// Single ε-closure (lookbehind-aware); reuses sc.eps stamp map.
		ethreads := epsilonCloseScratch(n, t, tokens, source, &sc.eps, &sc.look)
		for _, cur := range ethreads {
			if cur.state == n.accept {
				score := captureScore(cur.caps)
				end := cur.pos
				if score > bestScore || (score == bestScore && end < bestEnd) {
					bestScore, bestEnd = score, end
					cp := cloneThread(cur)
					best = &cp
				}
			}
			if cur.pos >= len(tokens) {
				continue
			}
			for _, e := range n.states[cur.state].edges {
				ok, named := predMatch(e.pred, tokens, cur.pos, source)
				if !ok {
					continue
				}
				// Own maps when ops or named regex groups will write captures.
				own := len(e.ops) > 0 || len(named) > 0
				last := cur.pos
				nt := stepThread(cur, e.to, cur.pos+1, own)
				if len(e.ops) > 0 {
					if err := applyOps(&nt, e.ops, tokens, source, last); err != nil {
						continue
					}
				}
				// regex named groups (append discipline; soft-fail on unify clash)
				reject := false
				for name, sp := range named {
					ensureCaps(&nt)
					if name == "\x00group" {
						// override last bind for captureGroup on primary name in ops
						for _, op := range e.ops {
							if op.kind != capBindRegex && op.kind != capAppendRegex {
								continue
							}
							if op.captureGroup <= 0 {
								continue
							}
							// replace last site: re-bind as same discipline
							if curCap, ok := nt.caps[op.name]; ok && len(curCap.Spans()) > 0 {
								sites := curCap.Spans()
								sites = sites[:len(sites)-1]
								switch curCap.(type) {
								case UnifySpan:
									nt.caps[op.name] = UnifySpan(sites)
								default:
									nt.caps[op.name] = AppendSpan(sites)
								}
							}
							unify := op.kind == capBindRegex && op.unify
							if err := bindInto(nt.caps, op.name, sp, source, unify); err != nil {
								reject = true
							}
						}
						continue
					}
					if err := bindInto(nt.caps, name, sp, source, false); err != nil {
						reject = true
					}
				}
				if reject {
					continue
				}
				k := statePosKey(nt.state, nt.pos)
				score := captureScore(nt.caps)
				if prev, ok := sc.seen[k]; ok && prev >= score {
					continue
				}
				sc.seen[k] = score
				sc.queue = append(sc.queue, nt)
			}
		}
	}

	if best == nil {
		return Match{}, startIdx, false, nil
	}
	// Match span: from startIdx token through last consumed.
	ms, me := tokens[startIdx].StartByte, tokens[startIdx].EndByte
	if best.pos > startIdx {
		me = tokens[best.pos-1].EndByte
	}
	_ = root
	return Match{
		Span:     ingestutil.Span{StartByte: ms, EndByte: me},
		Captures: flattenCapMap(best.caps),
	}, best.pos, true, nil
}

// epsilonClose is the allocating entry (tests / rare paths).
func epsilonClose(n *nfa, t nfaThread, tokens []tok, source []byte) []nfaThread {
	var eps, look epsScratch
	// Copy out before scratch is reused.
	out := epsilonCloseScratch(n, t, tokens, source, &eps, &look)
	cp := make([]nfaThread, len(out))
	copy(cp, out)
	return cp
}

func epsilonCloseScratch(n *nfa, t nfaThread, tokens []tok, source []byte, eps, look *epsScratch) []nfaThread {
	eps.bumpGen()
	eps.stack = eps.stack[:0]
	eps.out = eps.out[:0]
	eps.stack = append(eps.stack, t)
	for len(eps.stack) > 0 {
		cur := eps.stack[len(eps.stack)-1]
		eps.stack = eps.stack[:len(eps.stack)-1]
		k := statePosKey(cur.state, cur.pos)
		if eps.stamp[k] == eps.gen {
			continue
		}
		eps.stamp[k] = eps.gen
		eps.out = append(eps.out, cur)
		for _, e := range n.states[cur.state].eps {
			if e.negLookbehind != nil {
				if nfaMatchesEndingAt(e.negLookbehind, tokens, cur.pos, source, look) {
					continue // positive lookbehind would match → invert fails
				}
			}
			// Lookbehind gates and empty-ops ε can share maps; ops require own copy.
			nt := stepThread(cur, e.to, cur.pos, len(e.ops) > 0)
			if len(e.ops) > 0 {
				if err := applyOps(&nt, e.ops, tokens, source, -1); err != nil {
					continue
				}
			}
			eps.stack = append(eps.stack, nt)
		}
	}
	return eps.out
}

// tokenConsumeBound is the longest start→accept token count, or -1 if unbounded.
func (n *nfa) tokenConsumeBound() int {
	if n == nil {
		return 0
	}
	if !n.tokenBoundKnown {
		n.maxTokenConsume = nfaMaxTokenConsume(n)
		n.tokenBoundKnown = true
	}
	return n.maxTokenConsume
}

// nfaMaxTokenConsume is the longest start→accept consume count. -1 if a consuming
// cycle can still reach accept (lookbehind must search every start).
func nfaMaxTokenConsume(n *nfa) int {
	if n == nil || len(n.states) == 0 {
		return 0
	}
	stateCount := len(n.states)
	const unreachable = -1
	longest := make([]int, stateCount)
	for i := range longest {
		longest[i] = unreachable
	}
	longest[n.start] = 0
	type weightedEdge struct{ from, to, weight int }
	weightedEdges := make([]weightedEdge, 0, stateCount*2)
	for i, st := range n.states {
		for _, e := range st.eps {
			weightedEdges = append(weightedEdges, weightedEdge{i, e.to, 0})
		}
		for _, e := range st.edges {
			weightedEdges = append(weightedEdges, weightedEdge{i, e.to, 1})
		}
	}
	for round := 0; round < stateCount; round++ {
		relaxed := false
		for _, e := range weightedEdges {
			if longest[e.from] == unreachable {
				continue
			}
			candidate := longest[e.from] + e.weight
			if longest[e.to] == unreachable || candidate > longest[e.to] {
				longest[e.to] = candidate
				relaxed = true
				if round == stateCount-1 && e.weight > 0 && nfaCanReach(n, e.to, n.accept) {
					return -1
				}
			}
		}
		if !relaxed {
			break
		}
	}
	if longest[n.accept] == unreachable {
		return 0
	}
	return longest[n.accept]
}

func nfaCanReach(n *nfa, from, to int) bool {
	if n == nil || from < 0 || to < 0 || from >= len(n.states) || to >= len(n.states) {
		return false
	}
	if from == to {
		return true
	}
	seen := make([]bool, len(n.states))
	stack := []int{from}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[s] {
			continue
		}
		seen[s] = true
		if s == to {
			return true
		}
		for _, e := range n.states[s].eps {
			stack = append(stack, e.to)
		}
		for _, e := range n.states[s].edges {
			stack = append(stack, e.to)
		}
	}
	return false
}

// nfaMatchesEndingAt reports whether sub can match any token span [start, endPos)
// ending at endPos. Bounded lookbehinds only try start in [endPos-max, endPos].
// look is optional reusable ε-scratch for nested closes (must not be the outer eps).
func nfaMatchesEndingAt(sub *nfa, tokens []tok, endPos int, source []byte, look *epsScratch) bool {
	if sub == nil {
		return false
	}
	if endPos < 0 {
		endPos = 0
	}
	if endPos > len(tokens) {
		endPos = len(tokens)
	}
	if look == nil {
		look = &epsScratch{}
	}
	if sub.lookbehindStarNot {
		return lookbehindStarNotEndsAt(tokens, endPos, source, sub.lookbehindStarNotFirst, sub.lookbehindStarNotStop)
	}
	firstStart := 0
	if maxTokens := sub.tokenConsumeBound(); maxTokens >= 0 {
		firstStart = endPos - maxTokens
		if firstStart < 0 {
			firstStart = 0
		}
	}
	for start := firstStart; start <= endPos; start++ {
		if nfaAcceptsAt(sub, tokens, start, endPos, source, look) {
			return true
		}
	}
	return false
}

// lookbehindStarNotEndsAt is (seq FIRST (* (not STOP))) ending at endPos.
// After FIRST, gap-repeat consumes any token except the dead pair (func and STOP).
func lookbehindStarNotEndsAt(tokens []tok, endPos int, source []byte, first, stop nfaPred) bool {
	if endPos <= 0 {
		return false
	}
	for start := endPos - 1; start >= 0; start-- {
		ok, _ := predMatch(first, tokens, start, source)
		if !ok {
			continue
		}
		if starNotRestConsumable(tokens, start+1, endPos, source, stop) {
			return true
		}
	}
	return false
}

func starNotRestConsumable(tokens []tok, from, endPos int, source []byte, stop nfaPred) bool {
	for pos := from; pos < endPos; pos++ {
		canGap, _ := predMatch(nfaPred{kind: predAnyExceptFunc}, tokens, pos, source)
		isStop, _ := predMatch(stop, tokens, pos, source)
		canBody := !isStop
		if !canGap && !canBody {
			return false
		}
	}
	return true
}

// nfaAcceptsAt is true if sub reaches accept with pos == endPos starting at startIdx.
func nfaAcceptsAt(sub *nfa, tokens []tok, startIdx, endPos int, source []byte, look *epsScratch) bool {
	if look == nil {
		look = &epsScratch{}
	}
	look.bumpAccept()
	look.acceptQueue = append(look.acceptQueue, nfaThread{state: sub.start, pos: startIdx})
	head := 0
	for head < len(look.acceptQueue) {
		t := look.acceptQueue[head]
		head++
		if head > 64 && head*2 > len(look.acceptQueue) {
			look.acceptQueue = append(look.acceptQueue[:0], look.acceptQueue[head:]...)
			head = 0
		}
		k := statePosKey(t.state, t.pos)
		if look.acceptStamp[k] == look.acceptGeneration {
			continue
		}
		look.acceptStamp[k] = look.acceptGeneration
		for _, cur := range epsilonCloseScratch(sub, t, tokens, source, look, nil) {
			if cur.state == sub.accept && cur.pos == endPos {
				return true
			}
			if cur.pos > endPos {
				continue
			}
			if cur.pos >= len(tokens) {
				continue
			}
			// Do not consume past the lookbehind end.
			if cur.pos >= endPos {
				continue
			}
			for _, e := range sub.states[cur.state].edges {
				ok, named := predMatch(e.pred, tokens, cur.pos, source)
				if !ok {
					continue
				}
				// Drop named-regex extras for lookbehind (reject not needed; ignore binds)
				_ = named
				last := cur.pos
				nt := stepThread(cur, e.to, cur.pos+1, len(e.ops) > 0)
				if nt.pos > endPos {
					continue
				}
				if len(e.ops) > 0 {
					if err := applyOps(&nt, e.ops, tokens, source, last); err != nil {
						continue
					}
				}
				look.acceptQueue = append(look.acceptQueue, nt)
			}
		}
	}
	return false
}

func captureScore(caps map[string]CapSpan) int {
	n := 0
	for _, v := range caps {
		if v != nil {
			n += len(v.Spans())
		}
	}
	return n
}

// matchPattern runs the compiled NFA from each start position (legacy Node entry).
func matchPattern(pat Node, tokens []tok, source []byte, root *sitter.Node) ([]Match, error) {
	core, err := NodeToPat(pat)
	if err != nil {
		return nil, err
	}
	return matchPat(core, tokens, source, root)
}

// matchPat compiles Core Pat then runs from each start position.
func matchPat(pat Pat, tokens []tok, source []byte, root *sitter.Node) ([]Match, error) {
	n, err := compilePat(pat)
	if err != nil {
		return nil, err
	}
	return matchCompiled(n, tokens, source, root)
}

// matchCompiled runs a precompiled NFA from each start position (greedy advance).
func matchCompiled(n *nfa, tokens []tok, source []byte, root *sitter.Node) ([]Match, error) {
	var out []Match
	var sc nfaScratch
	for i := 0; i < len(tokens); {
		if len(n.startPreds) > 0 && !tokenMayStart(n.startPreds, tokens, i, source) {
			i++
			continue
		}
		m, next, ok, err := matchNFAScratch(n, tokens, i, source, root, &sc)
		if err != nil {
			return nil, err
		}
		if !ok {
			i++
			continue
		}
		out = append(out, m)
		if next <= i {
			i++
		} else {
			i = next
		}
	}
	return out, nil
}

// analyzeStartPreds collects first consuming preds reachable from start by ε.
// Returns nil when filtering is unsafe (empty match, lookbehind, any-token, …).
func analyzeStartPreds(n *nfa, isAccept func(int) bool) []nfaPred {
	if n == nil || len(n.states) == 0 || isAccept == nil {
		return nil
	}
	seen := make([]bool, len(n.states))
	stack := []int{n.start}
	var preds []nfaPred
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if s < 0 || s >= len(n.states) || seen[s] {
			continue
		}
		seen[s] = true
		// Zero-width success from start → must try every position.
		if isAccept(s) {
			return nil
		}
		st := n.states[s]
		for _, e := range st.eps {
			if e.negLookbehind != nil {
				return nil // position-dependent gate
			}
			stack = append(stack, e.to)
		}
		for _, e := range st.edges {
			if !predFilterable(e.pred) {
				return nil
			}
			preds = append(preds, e.pred)
		}
	}
	if len(preds) == 0 {
		return nil
	}
	return dedupeStartPreds(preds)
}

func predFilterable(p nfaPred) bool {
	switch p.kind {
	case predLit, predToken, predRef, predEquals:
		return true
	case predRegex:
		// Invert regex (not (regex …)) is a negative constraint; keep full NFA.
		return !p.invert && p.regex != nil
	default:
		// any / captureAny / anyExceptFunc / unknown → no filter
		return false
	}
}

func dedupeStartPreds(preds []nfaPred) []nfaPred {
	if len(preds) <= 1 {
		return preds
	}
	type key struct {
		kind         predKind
		text, ref    string
		equals       string
		invert       bool
		captureGroup int
	}
	seen := make(map[key]bool, len(preds))
	out := preds[:0]
	for _, p := range preds {
		k := key{
			kind: p.kind, text: p.text, ref: p.ref, equals: p.equals,
			invert: p.invert, captureGroup: p.captureGroup,
		}
		// Dedup regex by pattern string (compiled NFA keeps one *Regexp per edge).
		if p.regex != nil {
			k.text = p.regex.String()
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	return out
}

// tokenMayStart reports whether tokens[i] matches any first-token predicate.
func tokenMayStart(preds []nfaPred, tokens []tok, i int, source []byte) bool {
	if i < 0 || i >= len(tokens) {
		return false
	}
	for _, p := range preds {
		ok, _ := predMatch(p, tokens, i, source)
		if ok {
			return true
		}
	}
	return false
}
