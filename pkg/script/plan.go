package script

import (
	"fmt"

	"github.com/lewtec/patlint/pkg/pattern"
)

// Plan is the single-spine compile of a Program:
//   - Groups: full-file leaves → collapsed MultiLeafNFA
//   - Unders: under(region, bodyLeaf) sharing region → region once + body multi
//   - Residual: everything else → MatchFileMatcher
//   - Builtins: non-tape hooks
//
// Report / rewrite stay handlers on sites.
type Plan struct {
	Groups    []SpineGroup
	Unders    []UnderGroup
	Residual  []int
	Builtins  []int
	NeedLinks bool
}

// SpineGroup is one full-file multi-leaf domain.
type SpineGroup struct {
	Lang     string
	FileKey  string
	Arms     []pattern.LeafArm
	Multi    *pattern.MultiLeafNFA
	NeedLink bool
}

// UnderGroup is under(region, body*) with a shared region and collapsed body multi.
type UnderGroup struct {
	Lang      string
	FileKey   string
	RegionKey string
	Region    *pattern.CompiledMatcher
	Arms      []pattern.LeafArm
	Multi     *pattern.MultiLeafNFA
	NeedLink  bool
}

type groupKey struct {
	lang string
	file string
}

type underKey struct {
	lang, file, region string
}

// CompilePlan builds the multi-leaf + under spine from prog.Actions.
func CompilePlan(prog *Program) *Plan {
	if prog == nil {
		return &Plan{}
	}
	p := &Plan{}
	by := map[groupKey]*SpineGroup{}
	var groupOrder []groupKey
	uby := map[underKey]*UnderGroup{}
	var underOrder []underKey

	for i, act := range prog.Actions {
		if act.Builtin != "" {
			p.Builtins = append(p.Builtins, i)
			continue
		}
		if act.Matcher == nil {
			continue
		}
		if act.Matcher.NeedsLinks() {
			p.NeedLinks = true
		}

		// 1) full-file leaf → multi group
		if arm, fileKey, ok := act.Matcher.AsFullFileLeafArm(i); ok {
			gk := groupKey{lang: act.Lang, file: fileKey}
			g, exists := by[gk]
			if !exists {
				g = &SpineGroup{Lang: act.Lang, FileKey: fileKey}
				by[gk] = g
				groupOrder = append(groupOrder, gk)
			}
			g.Arms = append(g.Arms, arm)
			if act.Matcher.NeedsLinks() {
				g.NeedLink = true
			}
			continue
		}

		// 2) under(region, bodyLeaf) → under group
		if ub, ok := act.Matcher.AsUnderLeafBody(i); ok {
			uk := underKey{lang: act.Lang, file: ub.FileKey, region: ub.RegionKey}
			g, exists := uby[uk]
			if !exists {
				g = &UnderGroup{
					Lang:      act.Lang,
					FileKey:   ub.FileKey,
					RegionKey: ub.RegionKey,
					Region:    ub.Region,
				}
				uby[uk] = g
				underOrder = append(underOrder, uk)
			}
			g.Arms = append(g.Arms, ub.Body)
			if act.Matcher.NeedsLinks() {
				g.NeedLink = true
			}
			continue
		}

		// 3) residual
		p.Residual = append(p.Residual, i)
	}

	for _, gk := range groupOrder {
		g := by[gk]
		if multi, err := pattern.CompileMultiLeafNFA(g.Arms); err == nil {
			g.Multi = multi
		}
		p.Groups = append(p.Groups, *g)
	}
	for _, uk := range underOrder {
		g := uby[uk]
		if multi, err := pattern.CompileMultiLeafNFA(g.Arms); err == nil {
			g.Multi = multi
		}
		p.Unders = append(p.Unders, *g)
	}
	return p
}

// EnsurePlan returns prog.plan, compiling once if needed.
func (prog *Program) EnsurePlan() *Plan {
	if prog == nil {
		return &Plan{}
	}
	if prog.plan == nil {
		prog.plan = CompilePlan(prog)
	}
	return prog.plan
}

// PlanSummary is a short debug string (group / under / residual counts).
func (p *Plan) PlanSummary() string {
	if p == nil {
		return "(nil plan)"
	}
	var b []byte
	b = fmt.Appendf(b, "spine_groups=%d under_groups=%d residual=%d builtins=%d need_links=%v\n",
		len(p.Groups), len(p.Unders), len(p.Residual), len(p.Builtins), p.NeedLinks)
	for i, g := range p.Groups {
		b = fmt.Appendf(b, "  group[%d] lang=%q fileKey=%q arms=%d need_links=%v collapsed=%v\n",
			i, g.Lang, g.FileKey, len(g.Arms), g.NeedLink, g.Multi != nil)
	}
	for i, g := range p.Unders {
		b = fmt.Appendf(b, "  under[%d] lang=%q fileKey=%q region=%q arms=%d need_links=%v collapsed=%v\n",
			i, g.Lang, g.FileKey, trunc(g.RegionKey, 60), len(g.Arms), g.NeedLink, g.Multi != nil)
	}
	if len(p.Residual) > 0 {
		b = fmt.Appendf(b, "  residual_actions=%v\n", p.Residual)
	}
	return string(b)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// acceptsFileKey reports whether rel matches a spine file key.
func acceptsFileKey(fileKey, rel string) bool {
	if fileKey == "" || fileKey == pattern.FileKeyAlways() {
		return true
	}
	const prefix = "glob:"
	if len(fileKey) >= len(prefix) && fileKey[:len(prefix)] == prefix {
		return pattern.MatchPathGlob(fileKey[len(prefix):], rel)
	}
	return false
}
