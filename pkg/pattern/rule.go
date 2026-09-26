package pattern

import (
	"fmt"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"strings"
)

// Rule is one site transform: match, then sexp emit → []project.Edit.
// rft rewrite is sugar for (rewrite MATCH EMIT).
//
// Apply stays on ingest (Edit / StageEdits / ApplyEdits). Planners differ; the
// site engine is Rule → Match → []Edit.
//
// Prefer Matcher (precompiled spine) when present; Pattern (leftover Node)
// remains for graph fallbacks. mv use-site renames use Rule + NFA.
type Rule struct {
	Pattern Node
	// Matcher, when set, is the compiled plan (path/under + precompiled leaf NFAs).
	Matcher *CompiledMatcher
	// Emit is the sexp emit tree (string | (slot) | (ref …) | (seq …)).
	Emit any
	// Take, when non-empty, emits one edit per span of that capture.
	Take string
}

// Valid reports whether r can be expanded.
func (r Rule) Valid() error {
	if r.Matcher == nil && r.Pattern.Kind == "" {
		return fmt.Errorf("%w: rule: empty pattern", ErrRule)
	}
	if r.Emit == nil {
		return fmt.Errorf("%w: rule: empty emit", ErrRule)
	}
	return nil
}

// NeedsLinks reports whether matching this rule needs ingest hyperlink targets.
func (r Rule) NeedsLinks() bool {
	if r.Matcher != nil {
		return r.Matcher.NeedsLinks()
	}
	return PatternNeedsLinks(r.Pattern)
}

// Edits turns matches + this rule's emit into project.Edit values.
func (r Rule) Edits(matches []Match, source []byte) ([]project.Edit, error) {
	if err := r.Valid(); err != nil {
		return nil, err
	}
	return EditsForEmit(matches, r.Emit, source, r.Take)
}

// MatchFile runs this rule's pattern on one parsed file.
func (r Rule) MatchFile(sess *project.Session, root, fileRel string, source []byte, rootNode *sitter.Node, result *project.Result) ([]Match, error) {
	if r.Matcher != nil {
		return MatchFileMatcher(sess, root, fileRel, source, rootNode, r.Matcher, result)
	}
	if r.Pattern.Kind == "" {
		return nil, fmt.Errorf("%w: rule: empty pattern", ErrRule)
	}
	return MatchFile(sess, root, fileRel, source, rootNode, r.Pattern, result)
}

// ExpandFile matches and builds edits for one file under root.
func (r Rule) ExpandFile(sess *project.Session, root, fileRel string, source []byte, rootNode *sitter.Node, result *project.Result) (matches []Match, edits []project.Edit, err error) {
	matches, err = r.MatchFile(sess, root, fileRel, source, rootNode, result)
	if err != nil {
		return nil, nil, err
	}
	if len(matches) == 0 {
		return matches, nil, nil
	}
	edits, err = r.Edits(matches, source)
	return matches, edits, err
}

// RuleFromOp builds a site Rule from a rewrite Op (fixtures / CLI).
// A precompiled Matcher is the site pattern; Core reparse is only for leftover Node.
func RuleFromOp(op Op) (Rule, error) {
	if op.Matcher == nil {
		if err := op.ResolvePatternIR(); err != nil {
			return Rule{}, err
		}
	}
	if op.Emit == nil {
		return Rule{}, fmt.Errorf("%w: rule: op has no emit", ErrRule)
	}
	r := Rule{
		Pattern: op.PatternIR,
		Matcher: op.Matcher,
		Emit:    op.Emit,
		Take:    op.Take,
	}
	if r.Matcher == nil && r.Pattern.Kind != "" {
		core, err := NodeToPat(r.Pattern)
		if err != nil {
			return Rule{}, err
		}
		cm, err := CompileMatcher(LeafM{Pat: core})
		if err != nil {
			return Rule{}, err
		}
		r.Matcher = cm
	}
	if err := r.Valid(); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// RuleFromStrings parses a sexp matcher and a sexp emit.
func RuleFromStrings(patternStr, emitStr string) (Rule, error) {
	mexpr, err := ParseMatcher(patternStr)
	if err != nil {
		return Rule{}, fmt.Errorf("pattern: %w", err)
	}
	cm, err := CompileMatcher(mexpr)
	if err != nil {
		return Rule{}, fmt.Errorf("pattern: %w", err)
	}
	pat, err := matcherPatternNode(cm)
	if err != nil {
		return Rule{}, fmt.Errorf("pattern: %w", err)
	}
	emit, err := ParseEmit(emitStr)
	if err != nil {
		return Rule{}, fmt.Errorf("emit: %w", err)
	}
	r := Rule{Pattern: pat, Matcher: cm, Emit: emit, Take: matcherTakeName(mexpr)}
	if err := r.Valid(); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// RefLeafRule matches tokens whose hyperlink target is targetRef and replaces
// that leaf token with newText.
func RefLeafRule(targetRef, newText string) (Rule, error) {
	ref := strings.TrimSpace(targetRef)
	ref = strings.TrimPrefix(ref, "@")
	if ref == "" {
		return Rule{}, fmt.Errorf("%w: RefLeafRule: empty target ref", ErrRule)
	}
	if newText == "" {
		return Rule{}, fmt.Errorf("%w: RefLeafRule: empty new text", ErrRule)
	}
	return RuleFromStrings(fmt.Sprintf(`(ref %q)`, ref), fmt.Sprintf("%q", newText))
}
