package datalog

import "github.com/lewtec/patlint/pkg/store"

// Arg is a variable (?x) or a constant.
type Arg struct {
	Var   string // without '?'; empty ⇒ Const
	Const string
}

// Var is a body/head variable.
func Var(name string) Arg { return Arg{Var: name} }

// Const is a ground value.
func Const(v string) Arg { return Arg{Const: v} }

// Lit is one body or head atom. Builtin names start with "$".
type Lit struct {
	Rel  string
	Args []Arg
	Neg  bool
}

// Clause is head :- body.
type Clause struct {
	Head Lit
	Body []Lit
}

// Program is a list of clauses. Order does not define strata; negation does.
type Program []Clause

// Host evaluates builtins. name has no leading '$'.
// Return one or more result tuples (columns after the bound inputs), or no rows.
type Host interface {
	Builtin(name string, args []string) [][]string
}

// Env is a variable substitution.
type Env map[string]string

func (a Arg) bind(env Env) (string, bool) {
	if a.Var == "" {
		return a.Const, true
	}
	v, ok := env[a.Var]
	return v, ok
}

func (a Arg) unify(val string, env Env) (Env, bool) {
	if a.Var == "" {
		return env, a.Const == val
	}
	if have, ok := env[a.Var]; ok {
		return env, have == val
	}
	next := env.clone()
	next[a.Var] = val
	return next, true
}

func (e Env) clone() Env {
	n := make(Env, len(e)+2)
	for k, v := range e {
		n[k] = v
	}
	return n
}

func (l Lit) ground(env Env) (store.Tuple, bool) {
	t := make(store.Tuple, len(l.Args))
	for i, a := range l.Args {
		v, ok := a.bind(env)
		if !ok {
			return nil, false
		}
		t[i] = v
	}
	return t, true
}
