package pattern

import (
	"fmt"
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
)

// MultiLeafNFA is a collapsed multi-ε-NFA: one start, ε into each arm, tagged
// accept states. Match semantics for each arm ID match running that arm's NFA
// alone (per-tag best thread by capture score, then shorter end).
//
// Scan policy (MatchFileMultiLeaves): independent greedy cursors per arm — same
// SpanSet as sequential MatchFileMatcher per arm — but each cursor position
// runs one multi simulation instead of one matchNFA per due arm.
type MultiLeafNFA struct {
	n     nfa
	start int
	// acceptTags[state] = arm IDs that treat this state as accept.
	acceptTags map[int][]int
	// armByID for callish / debug.
	armByID map[int]LeafArm
	// ordered IDs (stable compile order).
	ids []int
}

// CompileMultiLeafNFA merges arm NFAs into one machine with a shared start and
// tagged accepts. Arms must be non-empty and each must have a non-nil nfa.
func CompileMultiLeafNFA(arms []LeafArm) (*MultiLeafNFA, error) {
	if len(arms) == 0 {
		return nil, fmt.Errorf("%w: multi-nfa: no arms", ErrMulti)
	}
	m := &MultiLeafNFA{
		acceptTags: map[int][]int{},
		armByID:    map[int]LeafArm{},
		ids:        make([]int, 0, len(arms)),
	}
	// Global start with only ε into each arm.
	m.start = 0
	m.n.states = []nfaState{{}} // q0
	m.n.start = 0
	// n.accept unused for multi (tagged map instead); set to -1.
	m.n.accept = -1

	for _, arm := range arms {
		if arm.nfa == nil || len(arm.nfa.states) == 0 {
			return nil, fmt.Errorf("%w: multi-nfa: arm id=%d has empty nfa", ErrMulti, arm.ID)
		}
		if _, dup := m.armByID[arm.ID]; dup {
			return nil, fmt.Errorf("%w: multi-nfa: duplicate arm id %d", ErrMulti, arm.ID)
		}
		aStart, aAccept := appendNFAStates(&m.n, arm.nfa)
		m.n.states[m.start].eps = append(m.n.states[m.start].eps, epsEdge{to: aStart})
		m.acceptTags[aAccept] = append(m.acceptTags[aAccept], arm.ID)
		m.armByID[arm.ID] = arm
		m.ids = append(m.ids, arm.ID)
	}
	m.n.startPreds = analyzeStartPreds(&m.n, func(s int) bool {
		return len(m.acceptTags[s]) > 0
	})
	return m, nil
}

// appendNFAStates copies src states into dst with remapped targets; returns
// start/accept indices in dst. Lookbehind sub-NFAs are shared (immutable).
func appendNFAStates(dst *nfa, src *nfa) (start, accept int) {
	off := len(dst.states)
	for _, st := range src.states {
		ns := nfaState{}
		for _, e := range st.eps {
			ns.eps = append(ns.eps, epsEdge{
				to:            e.to + off,
				ops:           e.ops,
				negLookbehind: e.negLookbehind,
			})
		}
		for _, e := range st.edges {
			ne := e
			ne.to = e.to + off
			ns.edges = append(ns.edges, ne)
		}
		dst.states = append(dst.states, ns)
	}
	return src.start + off, src.accept + off
}

// armHit is the best solo-equivalent hit for one arm from a start index.
type armHit struct {
	id    int
	match Match
	next  int
}

// matchMultiAt runs the collapsed machine from startIdx and returns the best
// hit per arm ID (same preference as matchNFA for that arm alone).
// Structure mirrors matchNFA; accept is tagged instead of a single accept state.
// sc may be nil.
func (m *MultiLeafNFA) matchMultiAt(tokens []tok, startIdx int, source []byte, root *sitter.Node) (map[int]armHit, error) {
	return m.matchMultiAtScratch(tokens, startIdx, source, root, nil)
}

func (m *MultiLeafNFA) matchMultiAtScratch(tokens []tok, startIdx int, source []byte, root *sitter.Node, sc *nfaScratch) (map[int]armHit, error) {
	if m == nil || len(m.n.states) == 0 {
		return nil, nil
	}
	n := &m.n
	if sc == nil {
		sc = &nfaScratch{}
	}
	if sc.seen == nil {
		sc.seen = make(map[uint64]int)
	} else {
		clear(sc.seen)
	}
	sc.queue = sc.queue[:0]

	type bestRec struct {
		th    nfaThread
		score int
		end   int
	}
	best := map[int]*bestRec{}

	consider := func(cur nfaThread) {
		tags := m.acceptTags[cur.state]
		if len(tags) == 0 {
			return
		}
		score := captureScore(cur.caps)
		end := cur.pos
		for _, id := range tags {
			prev := best[id]
			if prev == nil || score > prev.score || (score == prev.score && end < prev.end) {
				cp := cloneThread(cur)
				best[id] = &bestRec{th: cp, score: score, end: end}
			}
		}
	}

	sc.queue = append(sc.queue, nfaThread{state: m.start, pos: startIdx})
	head := 0

	for head < len(sc.queue) {
		t := sc.queue[head]
		head++
		if head > 64 && head*2 > len(sc.queue) {
			sc.queue = append(sc.queue[:0], sc.queue[head:]...)
			head = 0
		}

		// Single ε-closure (same as matchNFA); reuses sc.eps stamp map.
		ethreads := epsilonCloseScratch(n, t, tokens, source, &sc.eps, &sc.look)
		for _, cur := range ethreads {
			consider(cur)
			if cur.pos >= len(tokens) {
				continue
			}
			for _, e := range n.states[cur.state].edges {
				ok, named := predMatch(e.pred, tokens, cur.pos, source)
				if !ok {
					continue
				}
				own := len(e.ops) > 0 || len(named) > 0
				last := cur.pos
				nt := stepThread(cur, e.to, cur.pos+1, own)
				if len(e.ops) > 0 {
					if err := applyOps(&nt, e.ops, tokens, source, last); err != nil {
						continue
					}
				}
				reject := false
				for name, sp := range named {
					ensureCaps(&nt)
					if name == "\x00group" {
						for _, op := range e.ops {
							if op.kind != capBindRegex && op.kind != capAppendRegex {
								continue
							}
							if op.captureGroup <= 0 {
								continue
							}
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

	out := map[int]armHit{}
	if startIdx < 0 || startIdx >= len(tokens) {
		return out, nil
	}
	for id, rec := range best {
		ms, me := tokens[startIdx].StartByte, tokens[startIdx].EndByte
		if rec.th.pos > startIdx {
			me = tokens[rec.th.pos-1].EndByte
		}
		match := Match{
			Span:     ingestutil.Span{StartByte: ms, EndByte: me},
			Captures: flattenCapMap(rec.th.caps),
		}
		if arm, ok := m.armByID[id]; ok && arm.callish && root != nil {
			if node := coveringNode(root, match.StartByte, match.EndByte); node != nil {
				match.Span = ingestutil.Span{StartByte: node.StartByte(), EndByte: node.EndByte()}
			}
		}
		out[id] = armHit{id: id, match: match, next: rec.th.pos}
	}
	return out, nil
}

// Format dumps the collapsed multi-ε-NFA (shared start, tagged accepts).
func (m *MultiLeafNFA) Format(full bool) string {
	if m == nil {
		return "(nil MultiLeafNFA)\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "MultiLeafNFA collapsed arms=%d states=%d start=%d\n",
		len(m.ids), len(m.n.states), m.start)
	fmt.Fprintf(&b, "  tags:")
	for _, id := range m.ids {
		arm := m.armByID[id]
		pat := ""
		if arm.Pat != nil {
			pat = FormatSexp(arm.Pat)
		}
		fmt.Fprintf(&b, " id=%d", id)
		_ = pat
	}
	b.WriteByte('\n')
	for _, id := range m.ids {
		arm := m.armByID[id]
		fmt.Fprintf(&b, "  tag id=%d callish=%v pat=%s\n", id, arm.callish, FormatSexp(arm.Pat))
	}
	if !full {
		// List accept states only
		for st, tags := range m.acceptTags {
			fmt.Fprintf(&b, "  ACCEPT q%d tags=%v\n", st, tags)
		}
		return b.String()
	}
	// Full edge dump with accept annotations
	for i, st := range m.n.states {
		mark := ""
		if i == m.start {
			mark += " START"
		}
		if tags := m.acceptTags[i]; len(tags) > 0 {
			mark += fmt.Sprintf(" ACCEPT%v", tags)
		}
		fmt.Fprintf(&b, "  q%d%s\n", i, mark)
		for _, e := range st.eps {
			extra := ""
			if e.negLookbehind != nil {
				extra = " assert_not_behind"
			}
			ops := formatCapOps(e.ops)
			if ops != "" {
				ops = " " + ops
			}
			fmt.Fprintf(&b, "    ε -> q%d%s%s\n", e.to, ops, extra)
		}
		for _, e := range st.edges {
			ops := formatCapOps(e.ops)
			if ops != "" {
				ops = " " + ops
			}
			fmt.Fprintf(&b, "    %s -> q%d%s\n", formatPred(e.pred), e.to, ops)
		}
	}
	return b.String()
}
