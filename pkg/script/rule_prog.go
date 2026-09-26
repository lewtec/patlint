package script

import (
	"strconv"

	"github.com/lewtec/patlint/pkg/datalog"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/store"
)

// RuleClauses is stratum 3 sugar: one Horn clause per (rule ID LEVEL MESSAGE …).
// Body is the finder builtin $rule(idx, ?s, ?e); head is finding.
func (prog *Program) RuleClauses(rel string) datalog.Program {
	if prog == nil {
		return nil
	}
	var out datalog.Program
	for i, act := range prog.Actions {
		if act.Report == nil || act.Builtin != "" {
			continue
		}
		idx := strconv.Itoa(i)
		out = append(out, datalog.Clause{
			Head: datalog.Lit{Rel: store.RelationFinding, Args: []datalog.Arg{
				datalog.Const(rel),
				datalog.Var("s"), datalog.Var("e"),
				datalog.Const(act.Report.ID),
				datalog.Const(string(act.Report.Level)),
				datalog.Const(act.Report.Message),
			}},
			Body: []datalog.Lit{{Rel: "$rule", Args: []datalog.Arg{
				datalog.Const(idx), datalog.Var("s"), datalog.Var("e"),
			}}},
		})
	}
	return out
}

// ruleHost is the finder oracle for $rule: precomputed spine sites → spans.
type ruleHost struct {
	prog  *Program
	sites map[int][]pattern.Match
}

func (h *ruleHost) Builtin(name string, args []string) [][]string {
	if name != "rule" || len(args) < 1 || h == nil || h.prog == nil {
		return nil
	}
	i, err := strconv.Atoi(args[0])
	if err != nil || i < 0 || i >= len(h.prog.Actions) {
		return nil
	}
	act := h.prog.Actions[i]
	var out [][]string
	for _, m := range h.sites[i] {
		sp := m.Span
		if act.Take != "" {
			cap, ok := m.CaptureFirst(act.Take)
			if !ok {
				continue
			}
			sp = cap
		}
		if sp.EndByte <= sp.StartByte {
			continue
		}
		out = append(out, []string{store.Itoa(sp.StartByte), store.Itoa(sp.EndByte)})
	}
	return out
}
