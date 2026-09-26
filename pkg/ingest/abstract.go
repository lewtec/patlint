package ingest

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/tape"
)

// Abstract term alphabet (clone / similarity fingerprint).
const (
	// TermLitPrefix is used as LIT(<source text>) so values stay visible
	// (e.g. LIT(42), LIT("ok")). Literal *identity* is usually secondary to
	// structure for clones, but keeping the value is useful for debugging and
	// exact-match tiers.
	TermLitPrefix = "LIT("
	// TermID is only a last-resort bucket; prefer surface text or @ref.
	TermID = "ID"
)

// AbstractOptions controls ProjectAbstract.
type AbstractOptions struct {
	// NumberHoles assigns %r1, %r2, … per BindingSite.Key in first-seen order.
	// When false, bound idents become their surface name.
	NumberHoles bool
}

// DefaultAbstractOptions returns NumberHoles: true.
func DefaultAbstractOptions() AbstractOptions {
	return AbstractOptions{NumberHoles: true}
}

// FormatLit wraps source literal text as LIT(...).
func FormatLit(sourceText string) string {
	return TermLitPrefix + sourceText + ")"
}

// FormatRef formats a product reference term (keeps the real ref string).
func FormatRef(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return TermID
	}
	// @ prefix matches pattern dialect "hyperlink" leaves.
	if strings.HasPrefix(target, "@") {
		return target
	}
	return "@" + target
}

// ProjectAbstract maps structural tape cells to an abstract token stream.
//
// Priority for names (idents and types):
//  1. local/param binding → %rN (when NumberHoles) via map[span]Binding
//  2. cell.Target product ref → @provider:path::Name (actual ref, incl. types)
//  3. unbound ident/type → surface text (int, MyStruct, err, …)
//
// Literals become LIT(<source text>). Keywords/ops/punct/const stay as text.
// filePath selects pack ClassifyLeaf for leaf roles.
func ProjectAbstract(cells []tape.Cell, source []byte, filePath string, bindings *BindingIndex, opts AbstractOptions) []string {
	if len(cells) == 0 {
		return nil
	}
	holeOf := map[string]int{}
	nextHole := 1
	out := make([]string, 0, len(cells))

	for _, c := range cells {
		text := c.Text(source)
		if text == "" && c.Empty() {
			continue
		}
		cls := c.TokenClass
		sp := ingestutil.Span{StartByte: c.StartByte, EndByte: c.EndByte}

		// Product hyperlink wins over local binding only when there is no local
		// def at this span (params/locals always bind first).
		if bindings != nil {
			if site, ok := bindings.Lookup(sp); ok && site.Key != "" {
				if opts.NumberHoles {
					n, ok := holeOf[site.Key]
					if !ok {
						n = nextHole
						nextHole++
						holeOf[site.Key] = n
					}
					out = append(out, fmt.Sprintf("%%r%d", n))
				} else {
					out = append(out, site.Name)
				}
				continue
			}
		}
		// Resolved product ref (functions, types, packages, …) — keep the real ref.
		if c.Target != "" {
			out = append(out, FormatRef(c.Target))
			continue
		}

		switch cls {
		case HLComment:
			continue
		case HLString, HLNumber:
			out = append(out, FormatLit(text))
			continue
		case HLIdent, HLType:
			// Unbound name/type with no product ref: keep surface text.
			if strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
			continue
		case HLKeyword, HLOp, HLPunct, HLConst:
			out = append(out, text)
			continue
		default:
			if strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
	}
	return out
}

// CellsUnderSpan returns tape cells fully contained in [start, end).
func CellsUnderSpan(cells []tape.Cell, start, end uint32) []tape.Cell {
	if end <= start {
		return nil
	}
	var out []tape.Cell
	for _, c := range cells {
		if c.StartByte >= start && c.EndByte <= end && c.EndByte > c.StartByte {
			out = append(out, c)
		}
	}
	return out
}

// AbstractUnit is one fingerprintable region (e.g. a function body).
type AbstractUnit struct {
	// Name is a human label (func name, or path#start-end).
	Name string
	// ingestutil.Span covers the unit in the source buffer.
	Span ingestutil.Span
	// Terms is the abstract token stream.
	Terms []string
}

// FormatTerms joins abstract terms with spaces for display / hashing.
func FormatTerms(terms []string) string {
	return strings.Join(terms, " ")
}

// FormatTermsSexp renders abstract terms as a Core pattern S-expression
// (seq of lits, (unify rN any) holes, and (ref "…") product refs).
// Suitable as the shared "pattern" line for duplicate clusters.
func FormatTermsSexp(terms []string) string {
	if len(terms) == 0 {
		return "(seq)"
	}
	var b strings.Builder
	b.WriteString("(seq")
	for _, t := range terms {
		b.WriteByte(' ')
		b.WriteString(termToSexpAtom(t))
	}
	b.WriteByte(')')
	return b.String()
}

func termToSexpAtom(t string) string {
	if strings.HasPrefix(t, "%r") {
		// %r1 → (unify r1 any); same hole reuses the unify name.
		name := strings.TrimPrefix(t, "%")
		if name == "" {
			name = "r"
		}
		return fmt.Sprintf("(unify %s any)", name)
	}
	if strings.HasPrefix(t, "@") {
		target := strings.TrimPrefix(t, "@")
		return fmt.Sprintf("(ref %s)", strconv.Quote(target))
	}
	if strings.HasPrefix(t, TermLitPrefix) && strings.HasSuffix(t, ")") {
		// LIT(value) → quoted source literal text
		inner := t[len(TermLitPrefix) : len(t)-1]
		return strconv.Quote(inner)
	}
	return strconv.Quote(t)
}

// AbstractFile is the engine behind Walker.AbstractFile. Product and tests call
// the Walker method so policy is not a separate argument.
func AbstractFile(ctx context.Context, policy PackQueries, sess *project.Session, root *sitter.Node, source []byte, filePath string, onlyFuncs bool) []AbstractUnit {
	return AbstractFileResult(ctx, policy, sess, root, source, filePath, onlyFuncs, nil)
}

// AbstractFileResult is the engine behind Walker.AbstractFileResult.
//
// onlyFuncs units = pack ScopeNodeTypes ∪ pack embed regions.
// Empty scope list → no host units.
func AbstractFileResult(ctx context.Context, policy PackQueries, sess *project.Session, root *sitter.Node, source []byte, filePath string, onlyFuncs bool, result *project.Result) []AbstractUnit {
	if root == nil || root.IsNull() {
		return nil
	}
	var out []AbstractUnit
	if onlyFuncs {
		out = append(out, abstractUnitsFromEmbedded(ctx, policy, sess, embedRegions(policy, filePath, source, root), filePath, result)...)
	}
	out = append(out, abstractUnitsFromTree(ctx, policy, sess, root, source, filePath, onlyFuncs, result)...)
	return out
}

func abstractUnitsFromEmbedded(ctx context.Context, policy PackQueries, sess *project.Session, regs []project.EmbeddedSource, filePath string, result *project.Result) []AbstractUnit {
	var out []AbstractUnit
	for _, reg := range regs {
		if len(reg.Source) == 0 {
			continue
		}
		lang := reg.Language
		if lang == "" {
			continue
		}
		hint := reg.FileHint
		if hint == "" {
			hint = filePath
		}
		pf, err := ingestutil.ParseSource(sess.Engine(), reg.Source, hint, lang)
		if err != nil || pf == nil || pf.Root == nil {
			if pf != nil {
				pf.Close()
			}
			continue
		}
		// Product refs for the host file still apply when paths match.
		units := abstractUnitsFromTree(ctx, policy, sess, pf.Root, reg.Source, hint, true, result)
		pf.Close()
		for _, u := range units {
			u.Span = ingestutil.Span{StartByte: u.Span.StartByte + reg.Offset, EndByte: u.Span.EndByte + reg.Offset}
			out = append(out, u)
		}
	}
	return out
}

func abstractUnitsFromTree(ctx context.Context, policy PackQueries, sess *project.Session, root *sitter.Node, source []byte, filePath string, onlyFuncs bool, result *project.Result) []AbstractUnit {
	if root == nil || root.IsNull() {
		return nil
	}
	bindings := (*BindingIndex)(nil)
	pol := tape.DefaultPolicy()
	hostLang := ""
	if lang, ok, err := attributeHost(policy, filePath); err == nil && ok {
		hostLang = lang
		bindings = LocalBindingsForLanguage(ctx, policy, sess, root, source, lang, filePath)
		if policy != nil {
			pol = tape.WithAtomicTexts(policy.AtomicSpans(lang))
		}
	}
	targets := TapeUseTargets(result, filePath)
	cells := tape.Build(root, source, pol, targets)
	for i := range cells {
		if cells[i].TokenClass == "" && hostLang != "" {
			cells[i].TokenClass = ClassifyLeafLanguage(policy, hostLang, cells[i].Type)
		}
	}
	opts := DefaultAbstractOptions()

	if !onlyFuncs {
		return []AbstractUnit{{
			Name:  filePath,
			Span:  ingestutil.Span{StartByte: root.StartByte(), EndByte: root.EndByte()},
			Terms: ProjectAbstract(cells, source, filePath, bindings, opts),
		}}
	}

	unitTypes := abstractUnitTypesForPath(policy, filePath)
	if len(unitTypes) == 0 {
		return nil
	}
	isUnit := map[string]bool{}
	for _, t := range unitTypes {
		isUnit[t] = true
	}

	var units []AbstractUnit
	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil || n.IsNull() {
			return
		}
		if isUnit[n.Type()] {
			name := abstractUnitName(n, source)
			body := n
			if b := ingestutil.ChildByField(n, "body"); b != nil {
				body = b
			}
			sp := ingestutil.Span{StartByte: body.StartByte(), EndByte: body.EndByte()}
			sub := CellsUnderSpan(cells, sp.StartByte, sp.EndByte)
			// Re-number holes per unit (fresh ProjectAbstract call).
			terms := ProjectAbstract(sub, source, filePath, bindings, opts)
			if len(terms) > 0 {
				units = append(units, AbstractUnit{Name: name, Span: sp, Terms: terms})
			}
			// Descend for nested units (closures / local funcs / arrows).
			for i := uint32(0); i < n.ChildCount(); i++ {
				walk(n.Child(i))
			}
			return
		}
		for i := uint32(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return units
}

func abstractUnitTypesForPath(policy PackQueries, filePath string) []string {
	lang, ok, err := attributeHost(policy, filePath)
	if err != nil || !ok || policy == nil {
		return nil
	}
	return policy.ScopeNodeTypes(lang)
}

func abstractUnitName(n *sitter.Node, source []byte) string {
	if n == nil {
		return "?"
	}
	if name := ingestutil.ChildByField(n, "name"); name != nil {
		return string(source[name.StartByte():name.EndByte()])
	}
	// HTML-ish: element → start_tag/self_closing_tag → tag_name.
	for _, tagKind := range []string{"start_tag", "self_closing_tag"} {
		if st := ingestutil.ChildByType(n, tagKind); st != nil {
			if tn := ingestutil.ChildByType(st, "tag_name"); tn != nil {
				return string(source[tn.StartByte():tn.EndByte()])
			}
		}
	}
	// Do not use bare "identifier" children: Nix function_expression stores the
	// parameter under field "universal", which is not the unit's name.
	return fmt.Sprintf("%s@%d", n.Type(), n.StartByte())
}
