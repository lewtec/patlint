package pattern

import (
	"context"
	"fmt"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/tape"
	"path/filepath"
	"strings"
)

// Matcher is the authoring tree for the spine (SPEC.md Pattern algebra §15).
// Eval → SpanSet ( []Match ). Leaf is Core Pat; path/under/and/or/not compose.
type Matcher interface {
	matcher()
}

// PathM is (path GLOB M…) — file path gate, optional body.
// Body nil = pure gate (for and/not only).
type PathM struct {
	Glob string
	Body Matcher // nil = pure gate
}

func (PathM) matcher() {}

// UnderM is (under R B…) — body only inside region hits of R.
type UnderM struct {
	Region Matcher
	Body   Matcher
}

func (UnderM) matcher() {}

// TakeM is (take name M) — match-side locus (SPEC.md Pattern algebra §16.4).
// Each hit's Span becomes the first span of capture name (must be bound in M).
// Used so (under (take "args" …) body) domains on the capture, not the whole match.
type TakeM struct {
	Name string
	Body Matcher
}

func (TakeM) matcher() {}

// matcherTakeName returns the first (take name …) in the matcher tree.
func matcherTakeName(m Matcher) string {
	if m == nil {
		return ""
	}
	switch x := m.(type) {
	case TakeM:
		return x.Name
	case UnderM:
		if n := matcherTakeName(x.Body); n != "" {
			return n
		}
		return matcherTakeName(x.Region)
	case PathM:
		return matcherTakeName(x.Body)
	case AndM:
		for _, it := range x.Items {
			if n := matcherTakeName(it); n != "" {
				return n
			}
		}
	case OrM:
		for _, it := range x.Items {
			if n := matcherTakeName(it); n != "" {
				return n
			}
		}
	case NotM:
		return matcherTakeName(x.Inner)
	case AsLangM:
		if n := matcherTakeName(x.Body); n != "" {
			return n
		}
		return matcherTakeName(x.Region)
	}
	return ""
}

// AndM is (and M…) — pure gates ∧ exactly one finder.
type AndM struct{ Items []Matcher }

func (AndM) matcher() {}

// OrM is (or M…) — union of finders or OR of pure gates.
type OrM struct{ Items []Matcher }

func (OrM) matcher() {}

// NotM is (not G) — pure gate only.
type NotM struct{ Inner Matcher }

func (NotM) matcher() {}

// LeafM is a Core Pat leaf.
type LeafM struct{ Pat Pat }

func (LeafM) matcher() {}

// NodeM is (node TYPE) — region finder: one site per host AST node of that type.
// Each hit binds tree-sitter named fields as captures (field name → child span),
// so --vars shows name/parameters/body/… without a Core body. See SPEC.md Places.
type NodeM struct{ Type string }

func (NodeM) matcher() {}

// AsLangM is (as-language ID REGION BODY) — reparse each REGION locus as language
// ID and run BODY on the embedded tape. Spans are reported in host file bytes.
// See SPEC.md Places.
type AsLangM struct {
	Lang   string
	Region Matcher
	Body   Matcher
}

func (AsLangM) matcher() {}

// File-key constants for multi-leaf spine grouping (AsFullFileLeafArm).
const (
	fileKeyAlways = ""         // any path
	fileKeySolo   = "\x00solo" // complex FilePred; do not fuse across actions
)

// CompiledMatcher is the efficient eval form: FilePred + Finder tree.
type CompiledMatcher struct {
	// Accept reports whether relPath may contain hits (hoisted path gates).
	// Nil means always accept.
	Accept func(relPath string) bool
	root   finder
	// leafPat is the sole Core Pat when the matcher is a pure leaf (optional).
	leafPat Pat
	// fileKey groups fusible full-file leaves: "" always, "glob:pat", or fileKeySolo.
	fileKey string
}

// AcceptsFile is true when the file path passes FilePred.
func (c *CompiledMatcher) AcceptsFile(relPath string) bool {
	if c == nil {
		return false
	}
	if c.Accept == nil {
		return true
	}
	return c.Accept(filepath.ToSlash(relPath))
}

// NeedsLinks reports whether any leaf needs hyperlink targets.
func (c *CompiledMatcher) NeedsLinks() bool {
	if c == nil {
		return false
	}
	return finderNeedsLinks(c.root)
}

// CaptureNames returns capture/unify names from all leaves.
func (c *CompiledMatcher) CaptureNames() []string {
	if c == nil {
		return nil
	}
	return finderCaptureNames(c.root)
}

// LeafPat returns the Core Pat when the matcher is exactly one leaf; else nil, false.
func (c *CompiledMatcher) LeafPat() (Pat, bool) {
	if c == nil || c.leafPat == nil {
		return nil, false
	}
	return c.leafPat, true
}

// CompileMatcher lowers Matcher to FilePred + Finder.
func CompileMatcher(m Matcher) (*CompiledMatcher, error) {
	if m == nil {
		return nil, fmt.Errorf("%w: matcher: nil", ErrMatcher)
	}
	pred, f, err := compileMatcher(m)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("%w: matcher: no finder body (pure gate only)", ErrMatcher)
	}
	cm := &CompiledMatcher{root: f, fileKey: fileKeyFromPred(pred)}
	if pred != nil && !isPredTrue(pred) {
		cm.Accept = pred.match
	}
	if leaf, ok := f.(*finderLeaf); ok {
		cm.leafPat = leaf.pat
	}
	return cm, nil
}

func fileKeyFromPred(p filePred) string {
	if p == nil || isPredTrue(p) {
		return fileKeyAlways
	}
	if g, ok := p.(predGlob); ok {
		return "glob:" + g.glob
	}
	// and/or/not of paths — still a FilePred, but not a single glob key.
	return fileKeySolo
}

// FileKey returns the multi-spine grouping key (empty = always accept path).
func (c *CompiledMatcher) FileKey() string {
	if c == nil {
		return fileKeySolo
	}
	return c.fileKey
}

// FileKeyAlways is the spine group key for unrestricted paths.
func FileKeyAlways() string { return fileKeyAlways }

// MatchPathGlob reports whether rel matches a path glob (same rules as path gates).
func MatchPathGlob(pattern, rel string) bool {
	return matchPathGlob(pattern, rel)
}

// fileScratch is the pack's perception of one parsed tree: tape + node index.
// Finders query it; they do not walk the grammar tree again.
type fileScratch struct {
	tokens    []tok
	built     bool
	pol       tape.Policy
	forLang   func(lang string) tape.Policy
	want      map[string]struct{}
	index     *nodeIndex
	indexRoot *sitter.Node
}

func (s *fileScratch) tape(root *sitter.Node, source []byte, fileRel string, result *project.Result, pol tape.Policy) []tok {
	if s == nil {
		return tokensFromTape(root, source, buildLinkTargets(result, fileRel), pol)
	}
	if !s.built {
		s.tokens, s.index = ingestPerception(root, source, s.pol, s.want, buildLinkTargets(result, fileRel))
		s.indexRoot = root
		s.built = true
	}
	return s.tokens
}

func (s *fileScratch) policyForLang(lang string) tape.Policy {
	if s != nil && s.forLang != nil {
		return s.forLang(lang)
	}
	return tape.DefaultPolicy()
}

func (s *fileScratch) nodes(root *sitter.Node) *nodeIndex {
	if s == nil || root == nil || root.IsNull() {
		return nil
	}
	if s.built && s.indexRoot == root {
		return s.index
	}
	if len(s.want) == 0 {
		return nil
	}
	return buildNodeIndex(root, s.want)
}

// MatchFileMatcher runs a compiled matcher on one file (full-file domain).
func MatchFileMatcher(ctx context.Context, sess *project.Session, root, fileRel string, source []byte, rootNode *sitter.Node, cm *CompiledMatcher, result *project.Result) ([]Match, error) {
	return matchFileMatcherPol(ctx, sess, root, fileRel, source, rootNode, cm, result, tape.DefaultPolicy(), nil)
}

// MatchFileMatcherPolicy is MatchFileMatcher with an explicit tape policy (pack as-atomic).
func MatchFileMatcherPolicy(ctx context.Context, sess *project.Session, root, fileRel string, source []byte, rootNode *sitter.Node, cm *CompiledMatcher, result *project.Result, pol tape.Policy) ([]Match, error) {
	return matchFileMatcherPol(ctx, sess, root, fileRel, source, rootNode, cm, result, pol, nil)
}

func matchFileMatcherPol(ctx context.Context, sess *project.Session, root, fileRel string, source []byte, rootNode *sitter.Node, cm *CompiledMatcher, result *project.Result, pol tape.Policy, scratch *fileScratch) ([]Match, error) {
	if cm == nil {
		return nil, fmt.Errorf("%w: matcher: nil compiled", ErrMatcher)
	}
	rel := filepath.ToSlash(fileRel)
	if !cm.AcceptsFile(rel) {
		return nil, nil
	}
	tokens := scratch.tape(rootNode, source, fileRel, result, pol)
	if len(tokens) == 0 {
		return nil, nil
	}
	mc := &matchCtx{
		ctx:      ctx,
		sess:     sess,
		relPath:  rel,
		source:   source,
		root:     rootNode,
		tokens:   tokens,
		domain:   ingestutil.Span{StartByte: 0, EndByte: uint32(len(source))},
		fullFile: true,
		scratch:  scratch,
	}
	_ = root
	out, err := cm.root.find(mc)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].File = fileRel
	}
	return out, nil
}

// --- compile ---

type filePred interface {
	match(relPath string) bool
}

type predAnd []filePred
type predOr []filePred
type predNot struct{ inner filePred }
type predGlob struct{ glob string }
type predTrue struct{}

func (predTrue) match(string) bool       { return true }
func (p predGlob) match(rel string) bool { return matchPathGlob(p.glob, rel) }
func (p predNot) match(rel string) bool  { return !p.inner.match(rel) }
func (p predAnd) match(rel string) bool {
	for _, x := range p {
		if !x.match(rel) {
			return false
		}
	}
	return true
}
func (p predOr) match(rel string) bool {
	for _, x := range p {
		if x.match(rel) {
			return true
		}
	}
	return false
}

func andPred(a, b filePred) filePred {
	if a == nil || isPredTrue(a) {
		return b
	}
	if b == nil || isPredTrue(b) {
		return a
	}
	var out predAnd
	if aa, ok := a.(predAnd); ok {
		out = append(out, aa...)
	} else {
		out = append(out, a)
	}
	if bb, ok := b.(predAnd); ok {
		out = append(out, bb...)
	} else {
		out = append(out, b)
	}
	return out
}

func isPredTrue(p filePred) bool {
	_, ok := p.(predTrue)
	return ok || p == nil
}

type finder interface {
	find(ctx *matchCtx) ([]Match, error)
}

// finderLeaf is a Core Pat precompiled to an NFA at CompileMatcher time.
type finderLeaf struct {
	pat     Pat
	nfa     *nfa
	callish bool
}
type finderUnder struct{ region, body finder }
type finderTake struct {
	name string
	body finder
}
type finderNode struct{ typ string }
type finderAsLang struct {
	lang         string
	region, body finder
}

// finderOr is residual composition: under/take/mixed arms, or path-gated leaves.
// Pure or-of-leaves still uses this with precompiled finderLeaf arms (independent
// scans; same SpanSet as sequential matchPat). A single shared multi-NFA graph is
// optional later if event tags preserve per-arm env/geometry.
type finderOr struct{ arms []finder }

type matchCtx struct {
	ctx      context.Context
	sess     *project.Session
	relPath  string
	source   []byte
	root     *sitter.Node
	tokens   []tok
	domain   ingestutil.Span
	fullFile bool
	scratch  *fileScratch
}

func compileMatcher(m Matcher) (filePred, finder, error) {
	if g, ok := asPureGate(m); ok {
		return g, nil, nil
	}
	switch x := m.(type) {
	case PathM:
		gp := filePred(predGlob{glob: x.Glob})
		if x.Body == nil {
			return gp, nil, nil
		}
		p, f, err := compileMatcher(x.Body)
		if err != nil {
			return nil, nil, err
		}
		return andPred(gp, p), f, nil
	case UnderM:
		if x.Region == nil || x.Body == nil {
			return nil, nil, fmt.Errorf("%w: matcher: under needs region and body", ErrMatcher)
		}
		pr, fr, err := compileMatcher(x.Region)
		if err != nil {
			return nil, nil, err
		}
		if fr == nil {
			return nil, nil, fmt.Errorf("%w: matcher: under region must be a finder", ErrMatcher)
		}
		pb, fb, err := compileMatcher(x.Body)
		if err != nil {
			return nil, nil, err
		}
		if fb == nil {
			return nil, nil, fmt.Errorf("%w: matcher: under body must be a finder", ErrMatcher)
		}
		return andPred(pr, pb), &finderUnder{region: fr, body: fb}, nil
	case TakeM:
		if x.Name == "" || x.Body == nil {
			return nil, nil, fmt.Errorf("%w: matcher: take needs name and body", ErrMatcher)
		}
		p, f, err := compileMatcher(x.Body)
		if err != nil {
			return nil, nil, err
		}
		if f == nil {
			return nil, nil, fmt.Errorf("%w: matcher: take body must be a finder", ErrMatcher)
		}
		return p, &finderTake{name: x.Name, body: f}, nil
	case NodeM:
		if x.Type == "" {
			return nil, nil, fmt.Errorf("%w: matcher: node needs a type", ErrMatcher)
		}
		return predTrue{}, &finderNode{typ: x.Type}, nil
	case AsLangM:
		if x.Lang == "" || x.Region == nil || x.Body == nil {
			return nil, nil, fmt.Errorf("%w: matcher: as-language wants lang region body", ErrMatcher)
		}
		pr, fr, err := compileMatcher(x.Region)
		if err != nil {
			return nil, nil, err
		}
		if fr == nil {
			return nil, nil, fmt.Errorf("%w: matcher: as-language region must be a finder", ErrMatcher)
		}
		pb, fb, err := compileMatcher(x.Body)
		if err != nil {
			return nil, nil, err
		}
		if fb == nil {
			return nil, nil, fmt.Errorf("%w: matcher: as-language body must be a finder", ErrMatcher)
		}
		return andPred(pr, pb), &finderAsLang{lang: x.Lang, region: fr, body: fb}, nil
	case AndM:
		return compileAnd(x.Items)
	case OrM:
		return compileOr(x.Items)
	case NotM:
		g, ok := asPureGate(x.Inner)
		if !ok {
			return nil, nil, fmt.Errorf("%w: matcher: not requires a pure gate (path / and|or|not of paths)", ErrMatcher)
		}
		return predNot{inner: g}, nil, nil
	case LeafM:
		if x.Pat == nil {
			return nil, nil, fmt.Errorf("%w: matcher: empty leaf", ErrMatcher)
		}
		leaf, err := newFinderLeaf(x.Pat)
		if err != nil {
			return nil, nil, err
		}
		return predTrue{}, leaf, nil
	default:
		return nil, nil, fmt.Errorf("%w: matcher: unknown %T", ErrMatcher, m)
	}
}

// newFinderLeaf checks and precompiles a Core leaf.
func newFinderLeaf(p Pat) (*finderLeaf, error) {
	if p == nil {
		return nil, fmt.Errorf("%w: matcher: empty leaf", ErrMatcher)
	}
	if err := CheckPat(p); err != nil {
		return nil, err
	}
	n, err := compilePat(p)
	if err != nil {
		return nil, err
	}
	return &finderLeaf{pat: p, nfa: n, callish: looksLikeCallPat(p)}, nil
}

func compileAnd(items []Matcher) (filePred, finder, error) {
	if len(items) == 0 {
		return nil, nil, fmt.Errorf("%w: matcher: empty and", ErrMatcher)
	}
	var pred filePred = predTrue{}
	var bodies []finder
	for _, it := range items {
		if g, ok := asPureGate(it); ok {
			pred = andPred(pred, g)
			continue
		}
		p, f, err := compileMatcher(it)
		if err != nil {
			return nil, nil, err
		}
		pred = andPred(pred, p)
		if f != nil {
			bodies = append(bodies, f)
		}
	}
	switch len(bodies) {
	case 0:
		return pred, nil, nil
	case 1:
		return pred, bodies[0], nil
	default:
		return nil, nil, fmt.Errorf("%w: matcher: and allows at most one finder body", ErrMatcher)
	}
}

func compileOr(items []Matcher) (filePred, finder, error) {
	if len(items) == 0 {
		return nil, nil, fmt.Errorf("%w: matcher: empty or", ErrMatcher)
	}
	// All pure gates → gate OR
	allGate := true
	for _, it := range items {
		if _, ok := asPureGate(it); !ok {
			allGate = false
			break
		}
	}
	if allGate {
		var ps predOr
		for _, it := range items {
			g, _ := asPureGate(it)
			ps = append(ps, g)
		}
		return ps, nil, nil
	}
	// All finders: OR of finders. File accept if any arm's path pred passes;
	// each arm re-checks its pred at runtime (finderPred). Nested or flattens.
	var arms []finder
	var preds []filePred
	for _, it := range items {
		p, f, err := compileMatcher(it)
		if err != nil {
			return nil, nil, err
		}
		if f == nil {
			return nil, nil, fmt.Errorf("%w: matcher: or mix of pure gates and finders unsupported; wrap gates with a body", ErrMatcher)
		}
		preds = append(preds, p)
		// Flatten nested or; keep path-gated arms as finderPred.
		if isPredTrue(p) {
			if nested, ok := f.(*finderOr); ok {
				arms = append(arms, nested.arms...)
				continue
			}
			arms = append(arms, f)
			continue
		}
		arms = append(arms, &finderPred{pred: p, inner: f})
	}
	// File-level accept: any arm pred
	var fileP filePred = predTrue{}
	if len(preds) > 0 {
		var o predOr
		for _, p := range preds {
			if p != nil && !isPredTrue(p) {
				o = append(o, p)
			}
		}
		if len(o) > 0 {
			fileP = o
		}
	}
	if len(arms) == 1 {
		return fileP, arms[0], nil
	}
	return fileP, &finderOr{arms: arms}, nil
}

// finderPred skips when file pred fails (used under or).
type finderPred struct {
	pred  filePred
	inner finder
}

func (f *finderPred) find(ctx *matchCtx) ([]Match, error) {
	if f.pred != nil && !isPredTrue(f.pred) && !f.pred.match(ctx.relPath) {
		return nil, nil
	}
	return f.inner.find(ctx)
}

// asPureGate returns a FilePred when m is only path/not/and/or of paths.
func asPureGate(m Matcher) (filePred, bool) {
	if m == nil {
		return nil, false
	}
	switch x := m.(type) {
	case PathM:
		if x.Body != nil {
			return nil, false
		}
		return predGlob{glob: x.Glob}, true
	case NotM:
		g, ok := asPureGate(x.Inner)
		if !ok {
			return nil, false
		}
		return predNot{inner: g}, true
	case AndM:
		var ps predAnd
		for _, it := range x.Items {
			g, ok := asPureGate(it)
			if !ok {
				return nil, false
			}
			ps = append(ps, g)
		}
		if len(ps) == 0 {
			return nil, false
		}
		return ps, true
	case OrM:
		var ps predOr
		for _, it := range x.Items {
			g, ok := asPureGate(it)
			if !ok {
				return nil, false
			}
			ps = append(ps, g)
		}
		if len(ps) == 0 {
			return nil, false
		}
		return ps, true
	default:
		return nil, false
	}
}

// --- eval ---

func (f *finderLeaf) find(ctx *matchCtx) ([]Match, error) {
	toks := ctx.tokens
	if !ctx.fullFile {
		toks = tokensInDomain(ctx.tokens, ctx.domain)
	}
	if len(toks) == 0 {
		return nil, nil
	}
	out, err := matchCompiled(f.nfa, toks, ctx.source, ctx.root)
	if err != nil {
		return nil, err
	}
	// Drop matches that extend outside domain (safety).
	if !ctx.fullFile {
		filtered := out[:0]
		for _, m := range out {
			if spanInDomain(m.Span, ctx.domain) {
				filtered = append(filtered, m)
			}
		}
		out = filtered
	}
	for i := range out {
		if f.callish && ctx.root != nil {
			if n := coveringNode(ctx.root, out[i].StartByte, out[i].EndByte); n != nil {
				sp := ingestutil.Span{StartByte: n.StartByte(), EndByte: n.EndByte()}
				if ctx.fullFile || spanInDomain(sp, ctx.domain) {
					out[i].Span = sp
				}
			}
		}
	}
	return out, nil
}

func (f *finderUnder) find(ctx *matchCtx) ([]Match, error) {
	regs, err := f.region.find(ctx)
	if err != nil {
		return nil, err
	}
	var out []Match
	for _, r := range regs {
		sub := *ctx
		sub.domain = r.Span
		sub.fullFile = false
		ms, err := f.body.find(&sub)
		if err != nil {
			return nil, err
		}
		// Findings are body hits; region captures merge then body (append).
		for _, m := range ms {
			out = append(out, MergeRegionCaptures(r, m))
		}
	}
	return out, nil
}

// finderTake rewrites each hit's Span to capture name (SPEC.md Pattern algebra §16.4 locus).
func (f *finderTake) find(ctx *matchCtx) ([]Match, error) {
	ms, err := f.body.find(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Match, 0, len(ms))
	for _, m := range ms {
		// Last site is the leaf (Type.method → method).
		sp, ok := m.CaptureLast(f.name)
		if !ok || sp.Empty() {
			// No locus — drop (take requires the capture).
			continue
		}
		m.Span = sp
		out = append(out, m)
	}
	return out, nil
}

func (f *finderNode) find(ctx *matchCtx) ([]Match, error) {
	if ctx == nil || ctx.root == nil || f.typ == "" {
		return nil, nil
	}
	if idx := ctx.scratch.nodes(ctx.root); idx != nil {
		return idx.lookup(f.typ, ctx.domain, ctx.fullFile), nil
	}
	var out []Match
	collectNodesOfType(ctx.root, f.typ, ctx.domain, ctx.fullFile, &out)
	return out, nil
}

func collectNodesOfType(n *sitter.Node, typ string, domain ingestutil.Span, fullFile bool, out *[]Match) {
	if n == nil || n.IsNull() {
		return
	}
	sp := ingestutil.Span{StartByte: n.StartByte(), EndByte: n.EndByte()}
	// Skip subtrees entirely outside the domain.
	if !fullFile && (sp.EndByte <= domain.StartByte || sp.StartByte >= domain.EndByte) {
		return
	}
	if n.Type() == typ {
		if fullFile || spanInDomain(sp, domain) {
			*out = append(*out, Match{Span: sp, Captures: nodeFieldCaptures(n)})
		}
	}
	for i := uint32(0); i < n.ChildCount(); i++ {
		collectNodesOfType(n.Child(i), typ, domain, fullFile, out)
	}
}

// nodeFieldCaptures binds each tree-sitter named field on n as a capture.
// Anonymous children (keywords, punctuation) are skipped. Same field name
// more than once appends sites (multi-site capture).
func nodeFieldCaptures(n *sitter.Node) map[string][]ingestutil.Span {
	if n == nil || n.IsNull() {
		return nil
	}
	var caps map[string][]ingestutil.Span
	for i := uint32(0); i < n.ChildCount(); i++ {
		field := n.FieldNameForChild(i)
		if field == "" {
			continue
		}
		c := n.Child(i)
		if c == nil || c.IsNull() {
			continue
		}
		if caps == nil {
			caps = make(map[string][]ingestutil.Span)
		}
		caps[field] = append(caps[field], ingestutil.Span{
			StartByte: c.StartByte(),
			EndByte:   c.EndByte(),
		})
	}
	return caps
}

func (f *finderAsLang) find(ctx *matchCtx) ([]Match, error) {
	if ctx == nil || f.region == nil || f.body == nil || f.lang == "" {
		return nil, nil
	}
	regs, err := f.region.find(ctx)
	if err != nil {
		return nil, err
	}
	var out []Match
	for _, r := range regs {
		if r.Empty() || int(r.EndByte) > len(ctx.source) || r.StartByte >= r.EndByte {
			continue
		}
		raw := ctx.source[r.StartByte:r.EndByte]
		content, base := stripEmbedDelims(raw, r.StartByte)
		if len(content) == 0 {
			continue
		}
		hint := embedFileHint(f.lang)
		pf, err := ingestutil.ParseSource(ctx.ctx, ctx.sess.Engine(), content, hint, f.lang)
		if err != nil || pf == nil || pf.Root == nil {
			if pf != nil {
				pf.Close()
			}
			continue
		}
		tokens := tokensFromTape(pf.Root, content, nil, ctx.scratch.policyForLang(f.lang))
		sub := matchCtx{
			ctx:      ctx.ctx,
			sess:     ctx.sess,
			relPath:  ctx.relPath,
			source:   content,
			root:     pf.Root,
			tokens:   tokens,
			domain:   ingestutil.Span{StartByte: 0, EndByte: uint32(len(content))},
			fullFile: true,
			scratch:  ctx.scratch,
		}
		ms, err := f.body.find(&sub)
		pf.Close()
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			out = append(out, offsetMatch(m, base))
		}
	}
	return out, nil
}

// stripEmbedDelims removes outer string quotes/backticks or Nix ”…” so an
// embedded language can parse the inner text. base is the host start of content.
func stripEmbedDelims(raw []byte, start uint32) (content []byte, base uint32) {
	if len(raw) >= 4 && raw[0] == '\'' && raw[1] == '\'' &&
		raw[len(raw)-2] == '\'' && raw[len(raw)-1] == '\'' {
		return raw[2 : len(raw)-2], start + 2
	}
	if len(raw) >= 2 {
		switch raw[0] {
		case '`', '"', '\'':
			if raw[len(raw)-1] == raw[0] {
				return raw[1 : len(raw)-1], start + 1
			}
		}
	}
	return raw, start
}

func embedFileHint(lang string, exts ...string) string {
	best := ""
	for _, ext := range exts {
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		if best == "" || len(ext) < len(best) {
			best = ext
		}
	}
	if best != "" {
		return "embed" + best
	}
	if lang == "" {
		return "embed"
	}
	return "embed." + lang
}

func offsetMatch(m Match, base uint32) Match {
	m.StartByte += base
	m.EndByte += base
	if len(m.Captures) == 0 {
		return m
	}
	caps := make(map[string][]ingestutil.Span, len(m.Captures))
	for k, ss := range m.Captures {
		cp := make([]ingestutil.Span, len(ss))
		for i, sp := range ss {
			cp[i] = ingestutil.Span{StartByte: sp.StartByte + base, EndByte: sp.EndByte + base}
		}
		caps[k] = cp
	}
	m.Captures = caps
	return m
}

func (f *finderOr) find(ctx *matchCtx) ([]Match, error) {
	var out []Match
	for _, a := range f.arms {
		ms, err := a.find(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, ms...)
	}
	return out, nil
}

func tokensInDomain(tokens []tok, dom ingestutil.Span) []tok {
	var out []tok
	for _, t := range tokens {
		if t.StartByte >= dom.StartByte && t.EndByte <= dom.EndByte {
			out = append(out, t)
		}
	}
	return out
}

func spanInDomain(sp, dom ingestutil.Span) bool {
	return sp.StartByte >= dom.StartByte && sp.EndByte <= dom.EndByte
}

func finderNeedsLinks(f finder) bool {
	switch x := f.(type) {
	case *finderLeaf:
		return PatNeedsLinks(x.pat)
	case *finderUnder:
		return finderNeedsLinks(x.region) || finderNeedsLinks(x.body)
	case *finderTake:
		return finderNeedsLinks(x.body)
	case *finderNode:
		return false
	case *finderAsLang:
		return finderNeedsLinks(x.region) || finderNeedsLinks(x.body)
	case *finderOr:
		for _, a := range x.arms {
			if finderNeedsLinks(a) {
				return true
			}
		}
	case *finderPred:
		return finderNeedsLinks(x.inner)
	}
	return false
}

func finderCaptureNames(f finder) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(finder)
	walk = func(f finder) {
		switch x := f.(type) {
		case *finderLeaf:
			for _, n := range patCaptureNames(x.pat) {
				if !seen[n] {
					seen[n] = true
					out = append(out, n)
				}
			}
		case *finderUnder:
			walk(x.region)
			walk(x.body)
		case *finderTake:
			walk(x.body)
		case *finderNode:
			// Field names are grammar-dynamic; runtime fills Match.Captures.
		case *finderAsLang:
			walk(x.region)
			walk(x.body)
		case *finderOr:
			for _, a := range x.arms {
				walk(a)
			}
		case *finderPred:
			walk(x.inner)
		}
	}
	walk(f)
	return out
}

func patCaptureNames(p Pat) []string {
	var out []string
	var walk func(Pat)
	walk = func(p Pat) {
		switch x := p.(type) {
		case Capture:
			if x.Name != "" && x.Name != "_" {
				out = append(out, x.Name)
			}
			walk(x.Body)
		case Unify:
			if x.Name != "" && x.Name != "_" {
				out = append(out, x.Name)
			}
			walk(x.Body)
		case Seq:
			for _, it := range x.Items {
				walk(it)
			}
		case Alt:
			for _, it := range x.Items {
				walk(it)
			}
		case Rep:
			walk(x.Body)
		case Group:
			walk(x.Body)
		case AssertNotBehind:
			walk(x.Body)
		case Not:
			walk(x.Inner)
		}
	}
	walk(p)
	return out
}

// matchPathGlob matches rel (slash paths) against pattern with ** support.
func matchPathGlob(pattern, rel string) bool {
	pattern = strings.TrimSpace(filepath.ToSlash(pattern))
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "./")
	if pattern == "" {
		return false
	}
	// Unanchored basename-style: "*.go" matches any path ending with .go segment rules
	if !strings.Contains(pattern, "/") && !strings.HasPrefix(pattern, "**") {
		base := rel
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		if matchGlobRec(pattern, base) {
			return true
		}
		// also allow full-path match of pattern as suffix segments
		return matchGlobRec("**/"+pattern, rel)
	}
	return matchGlobRec(pattern, rel)
}

// matchGlobRec is a minimal ** / * / ? glob (path segments).
func matchGlobRec(pattern, name string) bool {
	for {
		if pattern == "" {
			return name == ""
		}
		if strings.HasPrefix(pattern, "**/") {
			rest := pattern[3:]
			if matchGlobRec(rest, name) {
				return true
			}
			for name != "" {
				if i := strings.IndexByte(name, '/'); i >= 0 {
					name = name[i+1:]
				} else {
					name = ""
				}
				if matchGlobRec(rest, name) {
					return true
				}
			}
			return false
		}
		if pattern == "**" {
			return true
		}
		if i := strings.IndexByte(pattern, '/'); i >= 0 {
			patSeg, patRest := pattern[:i], pattern[i+1:]
			if name == "" {
				return false
			}
			var nameSeg, nameRest string
			if j := strings.IndexByte(name, '/'); j >= 0 {
				nameSeg, nameRest = name[:j], name[j+1:]
			} else {
				nameSeg, nameRest = name, ""
			}
			if !matchSeg(patSeg, nameSeg) {
				return false
			}
			pattern, name = patRest, nameRest
			continue
		}
		// last pattern segment
		if strings.Contains(name, "/") {
			return false
		}
		return matchSeg(pattern, name)
	}
}

func matchSeg(pat, name string) bool {
	// * and ? only
	pi, ni := 0, 0
	star := -1
	match := 0
	for ni < len(name) {
		if pi < len(pat) && (pat[pi] == '?' || pat[pi] == name[ni]) {
			pi++
			ni++
			continue
		}
		if pi < len(pat) && pat[pi] == '*' {
			star = pi
			match = ni
			pi++
			continue
		}
		if star >= 0 {
			pi = star + 1
			match++
			ni = match
			continue
		}
		return false
	}
	for pi < len(pat) && pat[pi] == '*' {
		pi++
	}
	return pi == len(pat)
}
