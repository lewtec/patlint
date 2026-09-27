package pattern

import (
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"strconv"
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/tape"
)

// Match is one successful pattern match in a file.
// It does not retain file contents; pass the file []byte at the call site
// (Stream OnMatch/OnFile, Span.Text, PublicCaptures, Instantiate).
type Match struct {
	File            string
	ingestutil.Span // root match range
	// Captures maps capture names to one or more source spans.
	// Single holes have len 1; Multi (*) holes list every site in the gap.
	Captures map[string][]ingestutil.Span
}

// CaptureFirst returns the first span for name, or a zero Span.
func (m Match) CaptureFirst(name string) (ingestutil.Span, bool) {
	ss := m.Captures[name]
	if len(ss) == 0 {
		return ingestutil.Span{}, false
	}
	return ss[0], true
}

// CaptureLast returns the last span for name (innermost / leaf name).
func (m Match) CaptureLast(name string) (ingestutil.Span, bool) {
	ss := m.Captures[name]
	if len(ss) == 0 {
		return ingestutil.Span{}, false
	}
	return ss[len(ss)-1], true
}

// tok is a significant leaf: a Span plus optional hyperlink target.
type tok struct {
	ingestutil.Span
	target string
}

// linkTargets is hyperlink targets keyed by tape span (same map shape as
// ingest.TapeUseTargets / Walker.BuildTape).
type linkTargets = map[tape.Span]string

// buildLinkTargets returns TapeUseTargets for fileRel, or nil when empty.
func buildLinkTargets(result *project.Result, fileRel string) linkTargets {
	tt := ingest.TapeUseTargets(result, fileRel)
	if len(tt) == 0 {
		return nil
	}
	return tt
}

// MatchFile finds matches using the shared structural tape (pkg/tape) + pattern NFA.
// Used by rft grep, rewrite, and lint pattern rules. pat is legacy Node IR;
// it is converted to Core Pat then compiled (see CompilePat).
func MatchFile(sess *project.Session, root, fileRel string, source []byte, rootNode *sitter.Node, pat Node, result *project.Result) ([]Match, error) {
	core, err := NodeToPat(pat)
	if err != nil {
		return nil, err
	}
	return MatchFilePat(sess, root, fileRel, source, rootNode, core, result)
}

// MatchFilePat is MatchFile for Core Pat (canonical IR).
// Token cells come from the structural tape (Walker.BuildTape / tape.Build).
func MatchFilePat(sess *project.Session, root, fileRel string, source []byte, rootNode *sitter.Node, pat Pat, result *project.Result) ([]Match, error) {
	return matchFilePatPol(sess, root, fileRel, source, rootNode, pat, result, tape.DefaultPolicy())
}

func matchFilePatPol(sess *project.Session, root, fileRel string, source []byte, rootNode *sitter.Node, pat Pat, result *project.Result, pol tape.Policy) ([]Match, error) {
	_ = sess
	_ = root
	tokens := tokensFromTape(rootNode, source, buildLinkTargets(result, fileRel), pol)
	if len(tokens) == 0 {
		return nil, nil
	}
	out, err := matchPat(pat, tokens, source, rootNode)
	if err != nil {
		return nil, err
	}
	callish := looksLikeCallPat(pat)
	for i := range out {
		out[i].File = fileRel
		if callish && rootNode != nil {
			if n := coveringNode(rootNode, out[i].StartByte, out[i].EndByte); n != nil {
				out[i].Span = ingestutil.Span{StartByte: n.StartByte(), EndByte: n.EndByte()}
			}
		}
	}
	return out, nil
}

func flattenPattern(pat Node) []Node {
	switch pat.Kind {
	case "call":
		seq := []Node{}
		if pat.Callee != nil {
			seq = append(seq, *pat.Callee)
		}
		seq = append(seq, Node{Kind: "lit", Text: "("})
		for i, a := range pat.Args {
			if a.Kind == "rest" {
				seq = append(seq, a)
				continue
			}
			if i > 0 {
				seq = append(seq, Node{Kind: "lit", Text: ","})
			}
			seq = append(seq, flattenPattern(a)...)
		}
		seq = append(seq, Node{Kind: "lit", Text: ")"})
		return seq
	case "seq":
		var seq []Node
		for _, a := range pat.Args {
			seq = append(seq, flattenPattern(a)...)
		}
		return seq
	default:
		return []Node{pat}
	}
}

func looksLikeCallSeq(seq []Node) bool {
	for _, s := range seq {
		if s.Kind == "lit" && s.Text == "(" {
			return true
		}
	}
	return false
}

func selectorStart(tokens []tok, pos int, source []byte) uint32 {
	return selectorSpan(tokens, pos, source).StartByte
}

func selectorSpan(tokens []tok, pos int, source []byte) ingestutil.Span {
	i := pos
	for i >= 2 && tokens[i-1].Eq(source, ".") {
		i -= 2
	}
	return ingestutil.Span{StartByte: tokens[i].StartByte, EndByte: tokens[pos].EndByte}
}

// coveringNode returns the smallest AST node covering [start,end).
// Walks only into children that contain the span (not the whole tree).
func coveringNode(root *sitter.Node, start, end uint32) *sitter.Node {
	if root == nil || root.IsNull() {
		return nil
	}
	if root.StartByte() > start || root.EndByte() < end {
		return nil
	}
	n := root
	for {
		var next *sitter.Node
		cc := n.ChildCount()
		for i := uint32(0); i < cc; i++ {
			c := n.Child(i)
			if c == nil || c.IsNull() {
				continue
			}
			if c.StartByte() <= start && c.EndByte() >= end {
				// Prefer innermost when several children cover (rare for trees).
				if next == nil || (c.EndByte()-c.StartByte()) < (next.EndByte()-next.StartByte()) {
					next = c
				}
			}
		}
		if next == nil {
			return n
		}
		n = next
	}
}

func tokensFromTape(rootNode *sitter.Node, source []byte, targets linkTargets, pol tape.Policy) []tok {
	cells := tape.Build(rootNode, source, pol, targets)
	out := make([]tok, len(cells))
	for i, c := range cells {
		out[i] = tok{
			Span:   ingestutil.Span{StartByte: c.StartByte, EndByte: c.EndByte},
			target: c.Target,
		}
	}
	return out
}

// tokenContentMap derives the regex/equals match surface from a token span in
// source. For string lits, content is unquoted; for other tokens, content is
// the raw source slice. srcOf maps each content byte to an offset within the
// token (0-based); closeOff is the exclusive end of content within the token.
// Absolute source offset = t.StartByte + srcOf[i].
// tokenContent returns the regex/equals surface for a token without building
// a srcOf map (sufficient for MatchString / equals).
func tokenContent(src []byte, t tok) string {
	raw := t.Bytes(src)
	if content, ok := unquoteLiteralContent(raw); ok {
		return content
	}
	return string(raw)
}

func tokenContentMap(src []byte, t tok) (content string, srcOf []int, closeOff int, quoted bool) {
	raw := t.Bytes(src)
	if content, srcOf, closeOff, ok := unquoteLiteralMap(raw); ok {
		return content, srcOf, closeOff, true
	}
	srcOf = make([]int, len(raw))
	for i := range raw {
		srcOf[i] = i
	}
	return string(raw), srcOf, len(raw), false
}

// unquoteLiteralContent is unquoteLiteralMap without the offset map (faster
// when only the content string is needed).
func unquoteLiteralContent(raw []byte) (content string, ok bool) {
	if len(raw) < 2 {
		return "", false
	}
	q := raw[0]
	if (q != '"' && q != '\'' && q != '`') || raw[len(raw)-1] != q {
		return "", false
	}
	if q == '`' {
		return string(raw[1 : len(raw)-1]), true
	}
	// Escape-aware path still needs a builder; reuse map helper for correctness.
	c, _, _, ok := unquoteLiteralMap(raw)
	return c, ok
}

// unquoteLiteralMap unquotes a string token and records, for each content byte,
// the starting offset within the raw token of the source bytes that produced it.
// closeOff is the raw index of the closing quote.
func unquoteLiteralMap(raw []byte) (content string, srcOf []int, closeOff int, ok bool) {
	if len(raw) < 2 {
		return "", nil, 0, false
	}
	q := raw[0]
	if (q != '"' && q != '\'' && q != '`') || raw[len(raw)-1] != q {
		return "", nil, 0, false
	}
	closeOff = len(raw) - 1
	if q == '`' {
		inner := raw[1:closeOff]
		srcOf = make([]int, len(inner))
		for i := range inner {
			srcOf[i] = 1 + i
		}
		return string(inner), srcOf, closeOff, true
	}
	var b strings.Builder
	i := 1
	for i < closeOff {
		if raw[i] == '\\' && i+1 < closeOff {
			escStart := i
			i++
			switch raw[i] {
			case 'n':
				b.WriteByte('\n')
				srcOf = append(srcOf, escStart)
			case 't':
				b.WriteByte('\t')
				srcOf = append(srcOf, escStart)
			case '\\', '"', '\'':
				b.WriteByte(raw[i])
				srcOf = append(srcOf, escStart)
			default:
				// Preserve unknown escapes as two content bytes, both mapped.
				b.WriteByte('\\')
				srcOf = append(srcOf, escStart)
				b.WriteByte(raw[i])
				srcOf = append(srcOf, i)
			}
			i++
			continue
		}
		srcOf = append(srcOf, i)
		b.WriteByte(raw[i])
		i++
	}
	return b.String(), srcOf, closeOff, true
}

// unquoteLiteral is the content-only helper (no offset map).
// Prefers strconv.Unquote for std Go string/raw/char lits; falls back to the
// offset-map unquoter for odd single-quoted spans tree-sitter may emit.
func unquoteLiteral(raw string) (string, bool) {
	if s, err := strconv.Unquote(raw); err == nil {
		return s, true
	}
	content, _, _, ok := unquoteLiteralMap([]byte(raw))
	return content, ok
}

// contentSpanToSource maps a half-open range in unquoted/raw content into an
// absolute source Span using srcOf (content byte → raw token offset) and
// tokenStart. closeOff is the exclusive raw end of content (closing quote or
// len(token)). ok is false when the content range is out of bounds.
func contentSpanToSource(tokenStart uint32, srcOf []int, closeOff, cs, ce int) (ingestutil.Span, bool) {
	if cs < 0 || ce < cs || ce > len(srcOf) {
		return ingestutil.Span{}, false
	}
	if cs == ce {
		// Empty match: zero-width at cs (or at content end before closeOff).
		var s uint32
		if cs < len(srcOf) {
			s = tokenStart + uint32(srcOf[cs])
		} else {
			s = tokenStart + uint32(closeOff)
		}
		return ingestutil.Span{StartByte: s, EndByte: s}, true
	}
	start := tokenStart + uint32(srcOf[cs])
	var end uint32
	if ce < len(srcOf) {
		// Exclusive end is the start of the next content byte's source.
		end = tokenStart + uint32(srcOf[ce])
	} else {
		end = tokenStart + uint32(closeOff)
	}
	if end < start {
		return ingestutil.Span{}, false
	}
	return ingestutil.Span{StartByte: start, EndByte: end}, true
}
