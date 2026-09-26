package pattern

import (
	"fmt"
	"strings"
)

// FormatNFAFromPat compiles Core Pat and returns a text dump of the ε-NFA.
func FormatNFAFromPat(p Pat) (string, error) {
	n, err := compilePat(p)
	if err != nil {
		return "", err
	}
	return formatNFA(n, ""), nil
}

// FormatLeafArm dumps one multi-spine arm (same NFA MatchFileMultiLeaves runs).
// full=true lists every edge; false only states/start/accept + pat.
func FormatLeafArm(arm LeafArm, full bool) string {
	var b strings.Builder
	pat := ""
	if arm.Pat != nil {
		pat = FormatSexp(arm.Pat)
	}
	fmt.Fprintf(&b, "arm id=%d callish=%v pat=%s\n", arm.ID, arm.callish, pat)
	if arm.nfa == nil {
		b.WriteString("  (nil nfa)\n")
		return b.String()
	}
	if full {
		b.WriteString(formatNFA(arm.nfa, "  "))
	} else {
		fmt.Fprintf(&b, "  ε-NFA states=%d start=%d accept=%d\n",
			len(arm.nfa.states), arm.nfa.start, arm.nfa.accept)
	}
	return b.String()
}

// FormatMultiLeaves dumps a multi-leaf spine group (the Run primitive).
func FormatMultiLeaves(arms []LeafArm, full bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "MatchFileMultiLeaves arms=%d\n", len(arms))
	for i, arm := range arms {
		fmt.Fprintf(&b, "  [%d] ", i)
		// FormatLeafArm first line has no indent; re-indent body.
		s := FormatLeafArm(arm, full)
		lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
		if len(lines) == 0 {
			continue
		}
		b.WriteString(lines[0])
		b.WriteByte('\n')
		for _, line := range lines[1:] {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// FormatCompiledPlan dumps the matcher plan (finder tree + leaf NFAs).
// Used by `rft debug nfa` for scripts and composite matchers.
func FormatCompiledPlan(cm *CompiledMatcher) string {
	return formatCompiledPlan(cm, true)
}

// FormatCompiledPlanSummary is FormatCompiledPlan without full edge listings
// (only leaf pat + state counts).
func FormatCompiledPlanSummary(cm *CompiledMatcher) string {
	return formatCompiledPlan(cm, false)
}

func formatCompiledPlan(cm *CompiledMatcher, fullNFA bool) string {
	if cm == nil {
		return "(nil CompiledMatcher)\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "file_pred=%v\n", cm.Accept != nil)
	if cm.leafPat != nil {
		fmt.Fprintf(&b, "leaf_pat=%s\n", FormatSexp(cm.leafPat))
	}
	b.WriteString("plan:\n")
	formatFinder(&b, cm.root, "  ", fullNFA)
	return b.String()
}

func formatFinder(b *strings.Builder, f finder, ind string, fullNFA bool) {
	if f == nil {
		fmt.Fprintf(b, "%s(nil)\n", ind)
		return
	}
	switch x := f.(type) {
	case *finderLeaf:
		fmt.Fprintf(b, "%sleaf callish=%v needs_links=%v pat=%s\n",
			ind, x.callish, PatNeedsLinks(x.pat), FormatSexp(x.pat))
		if x.nfa != nil {
			if fullNFA {
				b.WriteString(formatNFA(x.nfa, ind+"  "))
			} else {
				fmt.Fprintf(b, "%s  ε-NFA states=%d start=%d accept=%d\n",
					ind, len(x.nfa.states), x.nfa.start, x.nfa.accept)
			}
		}
	case *finderUnder:
		fmt.Fprintf(b, "%sunder\n", ind)
		fmt.Fprintf(b, "%s  region:\n", ind)
		formatFinder(b, x.region, ind+"    ", fullNFA)
		fmt.Fprintf(b, "%s  body:\n", ind)
		formatFinder(b, x.body, ind+"    ", fullNFA)
	case *finderTake:
		fmt.Fprintf(b, "%stake %q\n", ind, x.name)
		formatFinder(b, x.body, ind+"  ", fullNFA)
	case *finderNode:
		fmt.Fprintf(b, "%snode %q\n", ind, x.typ)
	case *finderAsLang:
		fmt.Fprintf(b, "%sas-language %q\n", ind, x.lang)
		fmt.Fprintf(b, "%s  region:\n", ind)
		formatFinder(b, x.region, ind+"    ", fullNFA)
		fmt.Fprintf(b, "%s  body:\n", ind)
		formatFinder(b, x.body, ind+"    ", fullNFA)
	case *finderOr:
		fmt.Fprintf(b, "%sor arms=%d\n", ind, len(x.arms))
		for i, a := range x.arms {
			fmt.Fprintf(b, "%s  [%d]\n", ind, i)
			formatFinder(b, a, ind+"    ", fullNFA)
		}
	case *finderPred:
		fmt.Fprintf(b, "%spred %s\n", ind, formatFilePred(x.pred))
		formatFinder(b, x.inner, ind+"  ", fullNFA)
	default:
		fmt.Fprintf(b, "%s%T\n", ind, f)
	}
}

func formatFilePred(p filePred) string {
	if p == nil || isPredTrue(p) {
		return "true"
	}
	switch x := p.(type) {
	case predGlob:
		return fmt.Sprintf("glob %q", x.glob)
	case predNot:
		return "not(" + formatFilePred(x.inner) + ")"
	case predAnd:
		parts := make([]string, len(x))
		for i, c := range x {
			parts[i] = formatFilePred(c)
		}
		return "and(" + strings.Join(parts, ", ") + ")"
	case predOr:
		parts := make([]string, len(x))
		for i, c := range x {
			parts[i] = formatFilePred(c)
		}
		return "or(" + strings.Join(parts, ", ") + ")"
	default:
		return fmt.Sprintf("%T", p)
	}
}

// formatNFA writes a human-readable ε-NFA listing (states, ε-edges, consume edges).
func formatNFA(n *nfa, ind string) string {
	if n == nil {
		return ind + "(nil nfa)\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%sε-NFA states=%d start=%d accept=%d\n", ind, len(n.states), n.start, n.accept)
	for i, st := range n.states {
		mark := ""
		if i == n.start {
			mark += " START"
		}
		if i == n.accept {
			mark += " ACCEPT"
		}
		fmt.Fprintf(&b, "%s  q%d%s\n", ind, i, mark)
		for _, e := range st.eps {
			extra := ""
			if e.negLookbehind != nil {
				extra = " assert_not_behind(sub-nfa)"
			}
			ops := formatCapOps(e.ops)
			if ops != "" {
				ops = " " + ops
			}
			fmt.Fprintf(&b, "%s    ε -> q%d%s%s\n", ind, e.to, ops, extra)
			if e.negLookbehind != nil {
				// Nested lookbehind machine (compact one level).
				sub := formatNFA(e.negLookbehind, ind+"      ")
				b.WriteString(sub)
			}
		}
		for _, e := range st.edges {
			ops := formatCapOps(e.ops)
			if ops != "" {
				ops = " " + ops
			}
			fmt.Fprintf(&b, "%s    %s -> q%d%s\n", ind, formatPred(e.pred), e.to, ops)
		}
	}
	return b.String()
}

func formatPred(p nfaPred) string {
	inv := ""
	if p.invert {
		inv = "!"
	}
	switch p.kind {
	case predAny:
		return "any"
	case predAnyExceptFunc:
		return "any_except_func"
	case predCaptureAny:
		return "any"
	case predLit:
		return fmt.Sprintf("%slit %q", inv, p.text)
	case predToken:
		return fmt.Sprintf("%stoken %q", inv, p.text)
	case predRef:
		return fmt.Sprintf("%sref %q", inv, p.ref)
	case predRegex:
		re := ""
		if p.regex != nil {
			re = p.regex.String()
		}
		return fmt.Sprintf("%sregex %q", inv, re)
	case predEquals:
		return fmt.Sprintf("%sequals %q", inv, p.equals)
	default:
		return fmt.Sprintf("pred(%d)", p.kind)
	}
}

func formatCapOps(ops []capOp) string {
	if len(ops) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ops))
	for _, op := range ops {
		u := ""
		if op.unify {
			u = "/unify"
		}
		switch op.kind {
		case capOpen:
			parts = append(parts, fmt.Sprintf("open(%s%s)", op.name, u))
		case capClose:
			parts = append(parts, fmt.Sprintf("close(%s%s)", op.name, u))
		case capAppend:
			parts = append(parts, fmt.Sprintf("append(%s)", op.name))
		case capBindToken:
			parts = append(parts, fmt.Sprintf("bind_token(%s%s)", op.name, u))
		case capBindRef:
			parts = append(parts, fmt.Sprintf("bind_ref(%s%s)", op.name, u))
		case capBindRegex:
			parts = append(parts, fmt.Sprintf("bind_regex(%s%s,g%d)", op.name, u, op.captureGroup))
		case capAppendToken:
			parts = append(parts, fmt.Sprintf("append_token(%s)", op.name))
		case capAppendRef:
			parts = append(parts, fmt.Sprintf("append_ref(%s)", op.name))
		case capAppendRegex:
			parts = append(parts, fmt.Sprintf("append_regex(%s,g%d)", op.name, op.captureGroup))
		default:
			parts = append(parts, fmt.Sprintf("op(%d,%s)", op.kind, op.name))
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// FormatNFADot returns Graphviz DOT for a Core Pat's ε-NFA (single machine).
func FormatNFADot(p Pat) (string, error) {
	n, err := compilePat(p)
	if err != nil {
		return "", err
	}
	return formatNFADot(n), nil
}

func formatNFADot(n *nfa) string {
	var b strings.Builder
	b.WriteString("digraph nfa {\n  rankdir=LR;\n  node [shape=circle];\n")
	fmt.Fprintf(&b, "  q%d [shape=doublecircle];\n", n.accept)
	fmt.Fprintf(&b, "  start [shape=point];\n  start -> q%d;\n", n.start)
	for i, st := range n.states {
		for _, e := range st.eps {
			label := "ε"
			if ops := formatCapOps(e.ops); ops != "" {
				label += " " + ops
			}
			if e.negLookbehind != nil {
				label += " !behind"
			}
			fmt.Fprintf(&b, "  q%d -> q%d [label=%q, style=dashed];\n", i, e.to, label)
		}
		for _, e := range st.edges {
			label := formatPred(e.pred)
			if ops := formatCapOps(e.ops); ops != "" {
				label += " " + ops
			}
			fmt.Fprintf(&b, "  q%d -> q%d [label=%q];\n", i, e.to, label)
		}
	}
	b.WriteString("}\n")
	return b.String()
}
