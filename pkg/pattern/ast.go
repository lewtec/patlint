package pattern

import "fmt"

// Core pattern AST (see SPEC.md Pattern algebra). Leftover Node IR converts into Pat.

// Pat is a Core pattern node.
type Pat interface {
	pat()
}

// --- predicates (unit) ---

// Any matches one tape cell.
type Any struct{}

func (Any) pat() {}

// Lit matches cell.text exactly.
type Lit struct{ Text string }

func (Lit) pat() {}

// Token matches grammar node text (legacy kind "token" / "type_token").
type Token struct{ Text string }

func (Token) pat() {}

// Regex matches cell.text against RE. Invert is unit not(regex).
// CaptureGroup / Equals are legacy emit stretch fields for string holes.
type Regex struct {
	RE           string
	Invert       bool
	CaptureGroup int
	Equals       string
	FromCapture  string
}

func (Regex) pat() {}

// Ref matches cell.target.
type Ref struct{ Target string }

func (Ref) pat() {}

// Not is unit invert of a single-cell predicate (Inner must be unit).
type Not struct{ Inner Pat }

func (Not) pat() {}

// --- control ---

// Seq is concatenation.
type Seq struct{ Items []Pat }

func (Seq) pat() {}

// Alt is alternation (ε-split).
type Alt struct{ Items []Pat }

func (Alt) pat() {}

// Rep quantifies Body.
//
//	LocalOptional true  → ?  (min=0,max=1, local optional; Min/Max ignored for policy)
//	otherwise gap-repeat with Min..Max; Max < 0 means unbounded.
type Rep struct {
	LocalOptional bool
	Min, Max      int
	Body          Pat
}

func (Rep) pat() {}

// Group sets geometry to covering AST node of the matched range.
// Body is usually Seq or Alt of arms.
type Group struct{ Body Pat }

func (Group) pat() {}

// AssertNotBehind is zero-width negative lookbehind.
type AssertNotBehind struct{ Body Pat }

func (AssertNotBehind) pat() {}

// --- environment folds ---

// Capture append-binds Name for each occurrence of Body.
type Capture struct {
	Name string
	Body Pat
}

func (Capture) pat() {}

// Unify unify-binds Name for each occurrence of Body.
type Unify struct {
	Name string
	Body Pat
}

func (Unify) pat() {}

// Rest is sugar for rep(*, any); kept as a constructor helper.
func Rest() Pat { return Rep{Min: 0, Max: -1, Body: Any{}} }

// CheckPat enforces Core name discipline and structural rules.
func CheckPat(p Pat) error {
	mode := map[string]bool{} // name → unify
	var walk func(Pat) error
	walk = func(p Pat) error {
		if p == nil {
			return fmt.Errorf("%w: pattern: nil pat", ErrCompile)
		}
		switch x := p.(type) {
		case Capture:
			if err := checkNameFold(mode, x.Name, false); err != nil {
				return err
			}
			return walk(x.Body)
		case Unify:
			if err := checkNameFold(mode, x.Name, true); err != nil {
				return err
			}
			return walk(x.Body)
		case Rep:
			if x.LocalOptional {
				if x.Min != 0 || x.Max != 1 {
					// allow zero Min/Max when LocalOptional; normalize not required
				}
			} else if x.Max >= 0 && x.Min > x.Max {
				return fmt.Errorf("%w: pattern: rep min %d > max %d", ErrCompile, x.Min, x.Max)
			}
			// Binder-around-rep is rejected: Capture/Unify whose body is Rep.
			// Detected when walking Capture/Unify bodies separately below via isRep.
			return walk(x.Body)
		case Not:
			if !isUnitPred(x.Inner) {
				return fmt.Errorf("%w: pattern: not requires a unit predicate", ErrCompile)
			}
			return walk(x.Inner)
		case Seq:
			for _, it := range x.Items {
				if err := walk(it); err != nil {
					return err
				}
			}
		case Alt:
			if len(x.Items) == 0 {
				return fmt.Errorf("%w: pattern: empty alt", ErrCompile)
			}
			for _, it := range x.Items {
				if err := walk(it); err != nil {
					return err
				}
			}
		case Group:
			return walk(x.Body)
		case AssertNotBehind:
			return walk(x.Body)
		case Any, Lit, Token, Regex, Ref:
			return nil
		default:
			return fmt.Errorf("%w: pattern: unknown pat %T", ErrCompile, p)
		}
		return nil
	}
	// Second pass: reject capture(n, rep(...)) / unify(n, rep(...))
	var walkBind func(Pat) error
	walkBind = func(p Pat) error {
		switch x := p.(type) {
		case Capture:
			if _, ok := x.Body.(Rep); ok {
				return fmt.Errorf("%w: pattern: capture %q around rep rejected in v0; use rep(q, capture(%s, …))", ErrCompile, x.Name, x.Name)
			}
			return walkBind(x.Body)
		case Unify:
			if _, ok := x.Body.(Rep); ok {
				return fmt.Errorf("%w: pattern: unify %q around rep rejected in v0; use rep(q, unify(%s, …))", ErrCompile, x.Name, x.Name)
			}
			return walkBind(x.Body)
		case Rep:
			return walkBind(x.Body)
		case Not:
			return walkBind(x.Inner)
		case Seq:
			for _, it := range x.Items {
				if err := walkBind(it); err != nil {
					return err
				}
			}
		case Alt:
			for _, it := range x.Items {
				if err := walkBind(it); err != nil {
					return err
				}
			}
		case Group:
			return walkBind(x.Body)
		case AssertNotBehind:
			return walkBind(x.Body)
		}
		return nil
	}
	if err := walk(p); err != nil {
		return err
	}
	return walkBind(p)
}

func checkNameFold(mode map[string]bool, name string, unify bool) error {
	if name == "" || name == "_" || name == "ROOT" {
		return nil
	}
	if prev, ok := mode[name]; ok && prev != unify {
		return fmt.Errorf("%w: pattern: name %q used as both $%s and %%%s (one discipline per name)", ErrCompile, name, name, name)
	}
	mode[name] = unify
	return nil
}

func isUnitPred(p Pat) bool {
	switch p.(type) {
	case Any, Lit, Token, Regex, Ref:
		return true
	case Not:
		return true
	default:
		return false
	}
}

// isUnboundedStar reports rep(*, p) gap-repeat with min 0.
func isUnboundedStar(r Rep) bool {
	return !r.LocalOptional && r.Min == 0 && r.Max < 0
}

// isPlus reports rep(+, p).
func isPlus(r Rep) bool {
	return !r.LocalOptional && r.Min == 1 && r.Max < 0
}
