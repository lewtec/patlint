package pattern

import (
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/tape"
)

// LeafArm is one precompiled full-file leaf on a multi-leaf spine.
// ID is an opaque tag (e.g. script action index) returned on hits.
type LeafArm struct {
	ID      int
	nfa     *nfa
	callish bool
	Pat     Pat
}

// TaggedMatch is one hit from a multi-leaf scan (independent greedy per arm).
type TaggedMatch struct {
	ID    int
	Match Match
}

// AsFullFileLeafArm returns a LeafArm when cm is a pure full-file leaf
// (no under/or/take). fileKey is "" for always-accept path, "glob:…" for a
// single path gate, or "" with ok=false when the matcher is not fusible.
func (cm *CompiledMatcher) AsFullFileLeafArm(id int) (arm LeafArm, fileKey string, ok bool) {
	if cm == nil {
		return LeafArm{}, "", false
	}
	leaf, isLeaf := cm.root.(*finderLeaf)
	if !isLeaf || leaf == nil || leaf.nfa == nil {
		return LeafArm{}, "", false
	}
	if cm.fileKey == fileKeySolo {
		return LeafArm{}, "", false
	}
	return LeafArm{
		ID:      id,
		nfa:     leaf.nfa,
		callish: leaf.callish,
		Pat:     leaf.pat,
	}, cm.fileKey, true
}

// MatchFileMultiLeaves runs several full-file leaf arms on one tape via a
// collapsed multi-ε-NFA (CompileMultiLeafNFA). Each arm keeps an independent
// greedy cursor (same SpanSet as MatchFileMatcher per arm alone). Results are
// unordered; callers should group by ID and process in action order.
func MatchFileMultiLeaves(
	sess *project.Session,
	root, fileRel string,
	source []byte,
	rootNode *sitter.Node,
	arms []LeafArm,
	result *project.Result,
) ([]TaggedMatch, error) {
	return MatchFileMultiLeavesNFA(sess, root, fileRel, source, rootNode, nil, arms, result)
}

// MatchFileMultiLeavesNFA is MatchFileMultiLeaves with an optional precompiled
// collapsed machine (from CompileMultiLeafNFA). If multi is nil it is built from arms.
func MatchFileMultiLeavesNFA(
	sess *project.Session,
	root, fileRel string,
	source []byte,
	rootNode *sitter.Node,
	multi *MultiLeafNFA,
	arms []LeafArm,
	result *project.Result,
) ([]TaggedMatch, error) {
	return MatchFileMultiLeavesTape(sess, root, fileRel, source, rootNode, multi, arms, result, tape.DefaultPolicy())
}

// MatchFileMultiLeavesTape is MatchFileMultiLeavesNFA with an explicit tape policy.
func MatchFileMultiLeavesTape(
	sess *project.Session,
	root, fileRel string,
	source []byte,
	rootNode *sitter.Node,
	multi *MultiLeafNFA,
	arms []LeafArm,
	result *project.Result,
	pol tape.Policy,
) ([]TaggedMatch, error) {
	_ = sess
	_ = root
	if len(arms) == 0 && multi == nil {
		return nil, nil
	}
	tokens := tokensFromTape(rootNode, source, buildLinkTargets(result, fileRel), pol)
	if len(tokens) == 0 {
		return nil, nil
	}
	if multi == nil {
		var err error
		multi, err = CompileMultiLeafNFA(arms)
		if err != nil {
			return nil, err
		}
	}
	out, err := matchMultiLeavesCollapsed(multi, tokens, source, rootNode)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Match.File = fileRel
	}
	return out, nil
}

// matchMultiLeavesCollapsed: independent greedy cursors per arm; at each min
// cursor position one multi simulation serves all due arms.
func matchMultiLeavesCollapsed(multi *MultiLeafNFA, tokens []tok, source []byte, root *sitter.Node) ([]TaggedMatch, error) {
	ids := multi.ids
	n := len(ids)
	// cursor by arm index in ids
	cursor := make([]int, n)
	idIndex := map[int]int{}
	for i, id := range ids {
		idIndex[id] = i
	}
	var out []TaggedMatch
	var sc nfaScratch

	for {
		pos := -1
		for i := 0; i < n; i++ {
			if cursor[i] < 0 {
				continue
			}
			if cursor[i] >= len(tokens) {
				cursor[i] = -1
				continue
			}
			if pos < 0 || cursor[i] < pos {
				pos = cursor[i]
			}
		}
		if pos < 0 {
			break
		}
		// Skip full multi simulation when no arm can start at this token.
		if len(multi.n.startPreds) > 0 && !tokenMayStart(multi.n.startPreds, tokens, pos, source) {
			for i := range cursor {
				if cursor[i] == pos {
					cursor[i] = pos + 1
				}
			}
			continue
		}
		// One collapsed simulation from pos for all arms; apply only to due arms.
		hits, err := multi.matchMultiAtScratch(tokens, pos, source, root, &sc)
		if err != nil {
			return nil, err
		}
		for i, id := range ids {
			if cursor[i] != pos {
				continue
			}
			if h, ok := hits[id]; ok {
				out = append(out, TaggedMatch{ID: id, Match: h.match})
				if h.next <= pos {
					cursor[i] = pos + 1
				} else {
					cursor[i] = h.next
				}
			} else {
				cursor[i] = pos + 1
			}
		}
	}
	_ = idIndex
	return out, nil
}
