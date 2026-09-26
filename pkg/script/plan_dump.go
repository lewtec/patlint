package script

import (
	"fmt"
	"strings"

	"github.com/lewtec/patlint/pkg/pattern"
)

// FormatPlan dumps the same spine primitive Run uses: multi-leaf groups,
// residual MatchFileMatcher actions, and builtins. full controls NFA edge detail.
// If idFilter is non-empty, only matching rule ids / builtins / arm labels are kept
// (groups with no remaining arms are omitted).
func FormatPlan(prog *Program, full bool, idFilter string) string {
	if prog == nil {
		return "(nil program)\n"
	}
	p := prog.EnsurePlan()
	var b strings.Builder
	fmt.Fprintf(&b, "# script %s  actions=%d\n", prog.Path, len(prog.Actions))
	b.WriteString(p.PlanSummary())
	b.WriteString("\n# runtime primitive (same as script.Run)\n")

	// --- multi-leaf groups ---
	for gi, g := range p.Groups {
		arms := g.Arms
		if idFilter != "" {
			arms = filterArms(prog, arms, idFilter)
			if len(arms) == 0 {
				continue
			}
		}
		fmt.Fprintf(&b, "\n## spine group[%d]  MatchFileMultiLeaves (collapsed multi-ε-NFA)\n", gi)
		fmt.Fprintf(&b, "lang=%q fileKey=%q need_links=%v arms=%d\n",
			g.Lang, g.FileKey, g.NeedLink, len(arms))
		// Prefer dumping the real collapsed machine when unfiltered and available.
		if idFilter == "" && g.Multi != nil && len(arms) == len(g.Arms) {
			b.WriteString(g.Multi.Format(full))
			// Handler index: tag id → rule metadata
			fmt.Fprintf(&b, "\n  # handlers (tag → action)\n")
			for _, arm := range arms {
				label := actionLabelAt(prog, arm.ID)
				fmt.Fprintf(&b, "  tag id=%d → %s", arm.ID, label)
				if act := prog.Actions[arm.ID]; act.Emit != nil {
					b.WriteString(" emit")
				}
				if act := prog.Actions[arm.ID]; act.Report != nil {
					fmt.Fprintf(&b, " report=%s", act.Report.ID)
				}
				b.WriteByte('\n')
			}
			continue
		}
		// Filtered or no multi: per-arm (still the same NFAs that were merged).
		if g.Multi != nil && idFilter != "" {
			fmt.Fprintf(&b, "(filter %q — showing matching arms; full collapse has %d arms)\n",
				idFilter, len(g.Arms))
		}
		for _, arm := range arms {
			label := actionLabelAt(prog, arm.ID)
			fmt.Fprintf(&b, "\n### arm action[%d] %s\n", arm.ID, label)
			writeHandlerMeta(&b, prog, arm.ID)
			b.WriteString(pattern.FormatLeafArm(arm, full))
		}
	}

	// --- under groups (region once + body multi) ---
	for ui, ug := range p.Unders {
		arms := ug.Arms
		if idFilter != "" {
			arms = filterArms(prog, arms, idFilter)
			if len(arms) == 0 {
				continue
			}
		}
		fmt.Fprintf(&b, "\n## under group[%d]  region + MatchFileMultiLeavesDomain\n", ui)
		fmt.Fprintf(&b, "lang=%q fileKey=%q need_links=%v arms=%d\n",
			ug.Lang, ug.FileKey, ug.NeedLink, len(arms))
		fmt.Fprintf(&b, "regionKey=%s\n", ug.RegionKey)
		if ug.Region != nil {
			fmt.Fprintf(&b, "region:\n")
			if full {
				b.WriteString(indentBlock(pattern.FormatCompiledPlan(ug.Region), "  "))
			} else {
				b.WriteString(indentBlock(pattern.FormatCompiledPlanSummary(ug.Region), "  "))
			}
		}
		if idFilter == "" && ug.Multi != nil && len(arms) == len(ug.Arms) {
			fmt.Fprintf(&b, "body multi:\n")
			b.WriteString(indentBlock(ug.Multi.Format(full), "  "))
			fmt.Fprintf(&b, "  # handlers (tag → action)\n")
			for _, arm := range arms {
				label := actionLabelAt(prog, arm.ID)
				fmt.Fprintf(&b, "  tag id=%d → %s\n", arm.ID, label)
			}
			continue
		}
		for _, arm := range arms {
			label := actionLabelAt(prog, arm.ID)
			fmt.Fprintf(&b, "\n### under body action[%d] %s\n", arm.ID, label)
			writeHandlerMeta(&b, prog, arm.ID)
			b.WriteString(pattern.FormatLeafArm(arm, full))
		}
	}

	// --- residual (complex) ---
	for _, ai := range p.Residual {
		act := prog.Actions[ai]
		label := actionLabel(act)
		if idFilter != "" && !idMatchLabel(label, idFilter) {
			continue
		}
		fmt.Fprintf(&b, "\n## residual action[%d] %s  MatchFileMatcher\n", ai, label)
		writeHandlerMeta(&b, prog, ai)
		if act.Matcher == nil {
			b.WriteString("(no matcher)\n")
			continue
		}
		fmt.Fprintf(&b, "needs_links=%v fileKey=%q\n", act.Matcher.NeedsLinks(), act.Matcher.FileKey())
		if full {
			b.WriteString(pattern.FormatCompiledPlan(act.Matcher))
		} else {
			b.WriteString(pattern.FormatCompiledPlanSummary(act.Matcher))
		}
	}

	// --- builtins ---
	for _, ai := range p.Builtins {
		act := prog.Actions[ai]
		label := actionLabel(act)
		if idFilter != "" && !idMatchLabel(label, idFilter) {
			continue
		}
		fmt.Fprintf(&b, "\n## builtin action[%d] %s\n", ai, label)
		fmt.Fprintf(&b, "builtin=%s (no tape NFA; not on multi-leaf spine)\n", act.Builtin)
	}

	return b.String()
}

func filterArms(prog *Program, arms []pattern.LeafArm, idFilter string) []pattern.LeafArm {
	var out []pattern.LeafArm
	for _, arm := range arms {
		if idMatchLabel(actionLabelAt(prog, arm.ID), idFilter) {
			out = append(out, arm)
		}
	}
	return out
}

func actionLabelAt(prog *Program, i int) string {
	if prog == nil || i < 0 || i >= len(prog.Actions) {
		return fmt.Sprintf("action-%d", i)
	}
	return actionLabel(prog.Actions[i])
}

func writeHandlerMeta(b *strings.Builder, prog *Program, i int) {
	if prog == nil || i < 0 || i >= len(prog.Actions) {
		return
	}
	act := prog.Actions[i]
	if act.Lang != "" {
		fmt.Fprintf(b, "lang=%s\n", act.Lang)
	}
	if act.Take != "" {
		fmt.Fprintf(b, "take=%s\n", act.Take)
	}
	if act.Emit != nil {
		fmt.Fprintf(b, "emit=yes\n")
	}
	if act.Report != nil {
		fmt.Fprintf(b, "report=%s %s\n", act.Report.Level, act.Report.ID)
	}
}

func actionLabel(act Action) string {
	if act.Report != nil && act.Report.ID != "" {
		return act.Report.ID
	}
	if act.Builtin != "" {
		return act.Builtin
	}
	return fmt.Sprintf("step-%d", act.Index)
}

func idMatchLabel(label, want string) bool {
	if strings.EqualFold(label, want) {
		return true
	}
	return strings.Contains(strings.ToLower(label), strings.ToLower(want))
}

func indentBlock(s, ind string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(ind)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
