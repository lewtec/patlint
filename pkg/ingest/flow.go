package ingest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lewtec/patlint/pkg/project"
)

// FlowInc is one scored increment inside a unit.
type FlowInc struct {
	StartByte uint32
	EndByte   uint32
	Class     string
	Depth     int
	Points    int
}

// FlowReport is Appendix B arithmetic over as-flow rows inside a unit span.
type FlowReport struct {
	StartByte uint32
	EndByte   uint32
	Score     int
	Incs      []FlowInc
}

// ScoreFlow scores flow rows whose span is a proper subset of [unitStart, unitEnd).
// Nesting depth is how many structural|hybrid rows properly contain the site
// and are themselves a proper subset of the unit.
func ScoreFlow(unitStart, unitEnd uint32, flows []project.FlowDef) FlowReport {
	rep := FlowReport{StartByte: unitStart, EndByte: unitEnd}
	if unitEnd <= unitStart {
		return rep
	}
	var inside []project.FlowDef
	for _, fl := range flows {
		if fl.EndByte <= fl.StartByte {
			continue
		}
		if fl.StartByte >= unitStart && fl.EndByte <= unitEnd &&
			(fl.StartByte > unitStart || fl.EndByte < unitEnd) {
			inside = append(inside, fl)
		}
	}
	sort.SliceStable(inside, func(i, j int) bool {
		if inside[i].StartByte != inside[j].StartByte {
			return inside[i].StartByte < inside[j].StartByte
		}
		if inside[i].EndByte != inside[j].EndByte {
			return inside[i].EndByte > inside[j].EndByte
		}
		return inside[i].Class < inside[j].Class
	})
	for _, fl := range inside {
		inc := FlowInc{
			StartByte: fl.StartByte,
			EndByte:   fl.EndByte,
			Class:     fl.Class,
			Points:    1,
		}
		if fl.Class == project.FlowStructural {
			for _, other := range inside {
				if other.Class != project.FlowStructural && other.Class != project.FlowHybrid {
					continue
				}
				if other.StartByte <= fl.StartByte && other.EndByte >= fl.EndByte &&
					(other.StartByte < fl.StartByte || other.EndByte > fl.EndByte) {
					inc.Depth++
				}
			}
			inc.Points += inc.Depth
		}
		rep.Score += inc.Points
		rep.Incs = append(rep.Incs, inc)
	}
	return rep
}

// FormatFlowMermaid renders a flowchart of increments under name.
func FormatFlowMermaid(name string, rep FlowReport) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	unitID := "U"
	label := escapeMermaid(fmt.Sprintf("%s  score=%d", name, rep.Score))
	fmt.Fprintf(&b, "  %s[\"%s\"]\n", unitID, label)
	parentOf := make([]int, len(rep.Incs))
	for i := range parentOf {
		parentOf[i] = -1
	}
	for i, inc := range rep.Incs {
		best := -1
		var bestSpan uint32
		for j, other := range rep.Incs {
			if j == i {
				continue
			}
			if other.StartByte <= inc.StartByte && other.EndByte >= inc.EndByte &&
				(other.StartByte < inc.StartByte || other.EndByte > inc.EndByte) {
				span := other.EndByte - other.StartByte
				if best < 0 || span < bestSpan {
					best = j
					bestSpan = span
				}
			}
		}
		parentOf[i] = best
	}
	for i, inc := range rep.Incs {
		id := fmt.Sprintf("F%d", i)
		extra := ""
		if inc.Depth > 0 {
			extra = fmt.Sprintf(" +%d nest", inc.Depth)
		}
		nlabel := escapeMermaid(fmt.Sprintf("%s +%d%s", inc.Class, inc.Points, extra))
		fmt.Fprintf(&b, "  %s[\"%s\"]\n", id, nlabel)
		parent := unitID
		if p := parentOf[i]; p >= 0 {
			parent = fmt.Sprintf("F%d", p)
		}
		fmt.Fprintf(&b, "  %s --> %s\n", parent, id)
	}
	if len(rep.Incs) == 0 {
		b.WriteString("  U --> Z[\"(no flow marks)\"]\n")
	}
	return b.String()
}

// FormatFlowDot renders Graphviz DOT for the same graph.
func FormatFlowDot(name string, rep FlowReport) string {
	var b strings.Builder
	b.WriteString("digraph flow {\n")
	fmt.Fprintf(&b, "  U [label=%q];\n", fmt.Sprintf("%s  score=%d", name, rep.Score))
	parentOf := make([]int, len(rep.Incs))
	for i := range parentOf {
		parentOf[i] = -1
	}
	for i, inc := range rep.Incs {
		best := -1
		var bestSpan uint32
		for j, other := range rep.Incs {
			if j == i {
				continue
			}
			if other.StartByte <= inc.StartByte && other.EndByte >= inc.EndByte &&
				(other.StartByte < inc.StartByte || other.EndByte > inc.EndByte) {
				span := other.EndByte - other.StartByte
				if best < 0 || span < bestSpan {
					best = j
					bestSpan = span
				}
			}
		}
		parentOf[i] = best
		extra := ""
		if inc.Depth > 0 {
			extra = fmt.Sprintf(" +%d nest", inc.Depth)
		}
		fmt.Fprintf(&b, "  F%d [label=%q];\n", i, fmt.Sprintf("%s +%d%s", inc.Class, inc.Points, extra))
	}
	if len(rep.Incs) == 0 {
		b.WriteString("  Z [label=\"(no flow marks)\"];\n  U -> Z;\n")
	}
	for i := range rep.Incs {
		if p := parentOf[i]; p >= 0 {
			fmt.Fprintf(&b, "  F%d -> F%d;\n", p, i)
		} else {
			fmt.Fprintf(&b, "  U -> F%d;\n", i)
		}
	}
	b.WriteString("}\n")
	return b.String()
}

func escapeMermaid(s string) string {
	s = strings.ReplaceAll(s, `"`, "#quot;")
	s = strings.ReplaceAll(s, "[", "(")
	s = strings.ReplaceAll(s, "]", ")")
	return s
}
