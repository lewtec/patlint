package datalog

import (
	"context"
	"strings"

	"github.com/lewtec/patlint/pkg/store"
)

// Eval runs p on s until a fixpoint. host may be nil when p has no builtins.
// Cancel is observed between strata and between fixpoint rounds.
func Eval(ctx context.Context, s *store.Store, p Program, host Host) error {
	if s == nil || len(p) == 0 {
		return nil
	}
	for _, group := range stratify(p) {
		if err := ctx.Err(); err != nil {
			return err
		}
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			n := 0
			for _, c := range group {
				n += fire(s, c, host)
			}
			if n == 0 {
				break
			}
		}
	}
	return nil
}

func fire(s *store.Store, c Clause, host Host) int {
	var added int
	walk(s, c.Body, 0, Env{}, host, func(env Env) {
		head, ok := c.Head.ground(env)
		if !ok {
			return
		}
		if s.Insert(c.Head.Rel, head) {
			added++
		}
	})
	return added
}

func walk(s *store.Store, body []Lit, i int, env Env, host Host, yield func(Env)) {
	if i == len(body) {
		yield(env)
		return
	}
	lit := body[i]
	if strings.HasPrefix(lit.Rel, "$") {
		walkBuiltin(s, body, i, env, host, yield)
		return
	}
	if lit.Neg {
		if exists(s, lit, env, host) {
			return
		}
		walk(s, body, i+1, env, host, yield)
		return
	}
	for _, row := range s.Rows(lit.Rel) {
		if len(row) != len(lit.Args) {
			continue
		}
		next := env
		ok := true
		for j, a := range lit.Args {
			next, ok = a.unify(row[j], next)
			if !ok {
				break
			}
		}
		if ok {
			walk(s, body, i+1, next, host, yield)
		}
	}
}

func walkBuiltin(s *store.Store, body []Lit, i int, env Env, host Host, yield func(Env)) {
	lit := body[i]
	name := strings.TrimPrefix(lit.Rel, "$")
	in := make([]string, 0, len(lit.Args))
	free := make([]int, 0, 2)
	for j, a := range lit.Args {
		if v, ok := a.bind(env); ok {
			in = append(in, v)
			continue
		}
		free = append(free, j)
	}
	if lit.Neg {
		if host != nil && len(free) == 0 {
			if len(host.Builtin(name, in)) > 0 {
				return
			}
		}
		walk(s, body, i+1, env, host, yield)
		return
	}
	if host == nil {
		return
	}
	for _, out := range host.Builtin(name, in) {
		next := env
		ok := true
		if len(free) == 0 {
			// all bound: success if builtin produced any row
			walk(s, body, i+1, next, host, yield)
			return
		}
		if len(out) != len(free) {
			continue
		}
		for k, j := range free {
			next, ok = lit.Args[j].unify(out[k], next)
			if !ok {
				break
			}
		}
		if ok {
			walk(s, body, i+1, next, host, yield)
		}
	}
}

func exists(s *store.Store, lit Lit, env Env, host Host) bool {
	found := false
	walk(s, []Lit{{Rel: lit.Rel, Args: lit.Args}}, 0, env, host, func(Env) {
		found = true
	})
	return found
}

func stratify(p Program) [][]Clause {
	// Positive-only first, then clauses that mention not, then the rest that
	// write finding/edit. Enough for ingest → binds → query.
	var pos, neg, fx []Clause
	for _, c := range p {
		switch {
		case c.Head.Rel == store.RelationFinding || c.Head.Rel == store.RelationEdit:
			fx = append(fx, c)
		case hasNeg(c):
			neg = append(neg, c)
		default:
			pos = append(pos, c)
		}
	}
	var out [][]Clause
	if len(pos) > 0 {
		out = append(out, pos)
	}
	if len(neg) > 0 {
		out = append(out, neg)
	}
	if len(fx) > 0 {
		out = append(out, fx)
	}
	return out
}

func hasNeg(c Clause) bool {
	for _, b := range c.Body {
		if b.Neg {
			return true
		}
	}
	return false
}
