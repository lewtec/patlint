package pattern

import (
	"fmt"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/tape"
)

// UnderLeafBody is a fusible under(region, bodyLeaf) plan fragment.
// RegionKey groups actions that share the same region matcher (and take).
type UnderLeafBody struct {
	RegionKey string
	Region    *CompiledMatcher // run once per file for the group
	Body      LeafArm          // body leaf (action id in Body.ID)
	FileKey   string           // path gate from the outer matcher
}

// AsUnderLeafBody reports whether cm is under(region, pure-leaf-body) and thus
// eligible for under-spine fusion. Region may be a leaf or take(name, leaf).
// Nested under / or / complex bodies stay residual.
func (cm *CompiledMatcher) AsUnderLeafBody(id int) (UnderLeafBody, bool) {
	if cm == nil {
		return UnderLeafBody{}, false
	}
	u, ok := cm.root.(*finderUnder)
	if !ok {
		return UnderLeafBody{}, false
	}
	bodyLeaf, ok := u.body.(*finderLeaf)
	if !ok || bodyLeaf.nfa == nil {
		return UnderLeafBody{}, false
	}
	regKey, regCM, ok := regionCompiled(u.region, cm.fileKey)
	if !ok {
		return UnderLeafBody{}, false
	}
	return UnderLeafBody{
		RegionKey: regKey,
		Region:    regCM,
		Body: LeafArm{
			ID:      id,
			nfa:     bodyLeaf.nfa,
			callish: bodyLeaf.callish,
			Pat:     bodyLeaf.pat,
		},
		FileKey: cm.fileKey,
	}, true
}

func regionCompiled(region finder, outerFileKey string) (key string, cm *CompiledMatcher, ok bool) {
	switch r := region.(type) {
	case *finderLeaf:
		if r.nfa == nil {
			return "", nil, false
		}
		key = "leaf:" + FormatSexp(r.pat)
		cm = &CompiledMatcher{root: r, leafPat: r.pat, fileKey: outerFileKey}
		// Region inherits path gate so AcceptsFile stays consistent.
		if outerFileKey != fileKeyAlways && outerFileKey != fileKeySolo {
			if len(outerFileKey) > 5 && outerFileKey[:5] == "glob:" {
				g := outerFileKey[5:]
				cm.Accept = predGlob{glob: g}.match
			}
		}
		return key, cm, true
	case *finderTake:
		inner, ok := r.body.(*finderLeaf)
		if !ok || inner.nfa == nil {
			return "", nil, false
		}
		key = fmt.Sprintf("take:%s:%s", r.name, FormatSexp(inner.pat))
		cm = &CompiledMatcher{
			root:    r,
			fileKey: outerFileKey,
		}
		if outerFileKey != fileKeyAlways && outerFileKey != fileKeySolo {
			if len(outerFileKey) > 5 && outerFileKey[:5] == "glob:" {
				cm.Accept = predGlob{glob: outerFileKey[5:]}.match
			}
		}
		return key, cm, true
	default:
		return "", nil, false
	}
}

// MatchFileMultiLeavesDomain is MatchFileMultiLeavesNFA restricted to tokens
// whose spans lie under domain (absolute bytes preserved). Used for under bodies.
func MatchFileMultiLeavesDomain(
	sess *project.Session,
	root, fileRel string,
	source []byte,
	rootNode *sitter.Node,
	multi *MultiLeafNFA,
	arms []LeafArm,
	domain ingestutil.Span,
	result *project.Result,
	pol tape.Policy,
) ([]TaggedMatch, error) {
	_ = root
	if multi == nil && len(arms) == 0 {
		return nil, nil
	}
	tokens := tokensFromTape(rootNode, source, buildLinkTargets(result, fileRel), pol)
	if len(tokens) == 0 {
		return nil, nil
	}
	toks := tokensInDomain(tokens, domain)
	if len(toks) == 0 {
		return nil, nil
	}
	if multi == nil {
		var err error
		multi, err = CompileMultiLeafNFA(arms)
		if err != nil {
			return nil, err
		}
	}
	out, err := matchMultiLeavesCollapsed(multi, toks, source, rootNode)
	if err != nil {
		return nil, err
	}
	// Drop matches that extend outside domain (safety; callish widen).
	filtered := out[:0]
	for _, tm := range out {
		if !spanInDomain(tm.Match.Span, domain) {
			continue
		}
		tm.Match.File = fileRel
		filtered = append(filtered, tm)
	}
	return filtered, nil
}

// MergeRegionCaptures appends region captures then body captures (same key =
// many names: outer Type then inner method). Last site is the leaf.
func MergeRegionCaptures(region, body Match) Match {
	if len(region.Captures) == 0 {
		return body
	}
	merged := make(map[string][]ingestutil.Span, len(region.Captures)+len(body.Captures))
	for k, v := range region.Captures {
		merged[k] = append(merged[k], v...)
	}
	for k, v := range body.Captures {
		merged[k] = append(merged[k], v...)
	}
	body.Captures = merged
	return body
}
