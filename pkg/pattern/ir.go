// Package pattern implements structural match/rewrite for rft grep / rft rewrite
// (sexp patterns, Node IR, and testdata/pattern fixtures).
//
// Design lock for the Core algebra (tape · pred · control · geometry · env),
// builder let, JSON sexpr, and migration plan: SPEC.md Pattern algebra.
//
// Site transforms use Rule (pattern + replacement) → Match → []project.Edit.
// Apply/stage stay in package ingest. Symbol-identity mv plans stay in ingest;
// they may emit site Rules (e.g. RefLeafRule) for use-site text without owning
// a second match engine.
package pattern

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/pelletier/go-toml/v2"
)

// Op is the fixture/CLI operation description (test.toml).
type Op struct {
	Mode        string  `json:"mode"` // grep | rewrite
	Lang        string  `json:"lang"`
	Description string  `json:"description,omitempty"`
	Pattern     string  `json:"pattern,omitempty"`
	Replacement *string `json:"replacement"` // sexp emit text; null for grep
	// PatternIR is compile output (sexp → Node) for callers that still need Node.
	// Not an authoring field. LoadOp requires pattern or pattern_sexp.
	PatternIR Node `json:"pattern_ir,omitempty"`
	// PatternSexp is Core Pat as JSON sexpr (lispjson lists). When set, it is
	// the match IR source of truth and is compiled into PatternIR on resolve.
	PatternSexp json.RawMessage `json:"pattern_sexp,omitempty"`
	// Emit is the parsed sexp emit tree. Not JSON.
	Emit any `json:"-"`
	// Take is the (take name …) locus from the matcher, if any. Not JSON.
	Take             string   `json:"-"`
	ExpectMatchCount *int     `json:"expect_match_count,omitempty"`
	Notes            []string `json:"notes,omitempty"`
	// Matcher is the compiled path/under/and/or/not spine. Not JSON.
	Matcher *CompiledMatcher `json:"-"`
}

// Node is one pattern/replacement IR node.
type Node struct {
	Kind string `json:"kind"`

	As  string `json:"as,omitempty"`
	Ref string `json:"ref,omitempty"`

	// token (grammar node text) / lit
	Text string `json:"text,omitempty"`

	// string
	Equals string `json:"equals,omitempty"`
	Regex  string `json:"regex,omitempty"`
	// Invert is true for:
	//   - (not (regex …)): token text must NOT match Regex
	//     (named/numbered groups are not taken; the full token binds on success)
	//   - (assert_not_behind …): zero-width **negative lookbehind** — the group
	//     (incl. alts) must NOT match any span ending at the current token index.
	//     Does not consume tokens; inner captures are not bound. Multi (* / +) illegal.
	Invert       bool   `json:"invert,omitempty"`
	CaptureGroup int    `json:"capture_group,omitempty"`
	FromCapture  string `json:"from_capture,omitempty"`

	// Multi (* or + or {n,m}): gap-repeat — collect matching sites in the gap
	// (non-adjacent ok). Works with Unify=false (append) or Unify=true (text-equal per site).
	Multi bool `json:"multi,omitempty"`
	// MultiPlus is true for + (one or more). False with Multi means * unless RepMax set.
	MultiPlus bool `json:"multi_plus,omitempty"`
	// MultiOptional is true for ? — local optional (p|ε), not gap-repeat.
	MultiOptional bool `json:"multi_optional,omitempty"`
	// RepMin/RepMax bound gap-repeat when Multi is set.
	// Max < 0 means unbounded. Zero-value Max with Multi and !MultiPlus means legacy *.
	// Parser always sets explicit bounds for * (0,-1) and + (1,-1).
	RepMin int `json:"rep_min,omitempty"`
	RepMax int `json:"rep_max,omitempty"`

	// Unify is true for (unify name …) (single value, later sites must equal text).
	// False for (capture name …) (append every bind).
	Unify bool `json:"unify,omitempty"`

	// call: Callee + Args
	// group: Callee = single-arm body; Args = alt arms when len(Args)>0.
	// Prefer Args.
	Callee *Node  `json:"callee,omitempty"`
	Args   []Node `json:"args,omitempty"`
}

// LoadOp reads a grep/rewrite fixture from test.toml or a case directory.
func LoadOp(path string) (Op, error) {
	st, err := os.Stat(path)
	if err != nil {
		return Op{}, err
	}
	if st.IsDir() {
		return loadOpTOML(lewpath.New(path, "test.toml").String())
	}
	return loadOpTOML(path)
}

type opTOML struct {
	Kind             string `toml:"kind"`
	Description      string `toml:"description"`
	Lang             string `toml:"lang"`
	Pattern          string `toml:"pattern"`
	Emit             string `toml:"emit"`
	ExpectMatchCount *int   `toml:"expect_match_count"`
}

func loadOpTOML(path string) (Op, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Op{}, err
	}
	var t opTOML
	if err := toml.Unmarshal(b, &t); err != nil {
		return Op{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if t.Kind != "grep" && t.Kind != "rewrite" {
		return Op{}, fmt.Errorf("%w: %s: kind must be grep or rewrite, got %q", ErrRule, path, t.Kind)
	}
	op := Op{
		Mode:             t.Kind,
		Lang:             t.Lang,
		Description:      t.Description,
		Pattern:          t.Pattern,
		ExpectMatchCount: t.ExpectMatchCount,
	}
	if t.Kind == "rewrite" {
		em := t.Emit
		op.Replacement = &em
	}
	return finishOp(path, op)
}

func finishOp(path string, op Op) (Op, error) {
	if op.Mode != "grep" && op.Mode != "rewrite" {
		return Op{}, fmt.Errorf("%w: %s: mode must be grep or rewrite, got %q", ErrRule, path, op.Mode)
	}
	if err := op.ResolvePatternIR(); err != nil {
		return Op{}, fmt.Errorf("%s: %w", path, err)
	}
	if op.Mode == "rewrite" {
		if err := op.PrepareRewrite(); err != nil {
			return Op{}, fmt.Errorf("%s: %w", path, err)
		}
	}
	return op, nil
}

// PrepareRewrite fills Emit, Take, and Matcher from Replacement and Pattern.
func (op *Op) PrepareRewrite() error {
	if op == nil {
		return fmt.Errorf("%w: nil op", ErrRule)
	}
	if op.Replacement == nil || strings.TrimSpace(*op.Replacement) == "" {
		return fmt.Errorf("%w: rewrite requires replacement emit", ErrRule)
	}
	emit, err := ParseEmit(*op.Replacement)
	if err != nil {
		return fmt.Errorf("emit: %w", err)
	}
	op.Emit = emit
	if m, err := ParseMatcher(op.Pattern); err == nil {
		op.Take = matcherTakeName(m)
		if cm, err := CompileMatcher(m); err == nil {
			op.Matcher = cm
		}
	}
	return nil
}

// ResolvePatternIR fills PatternIR from PatternSexp when present.
// PatternSexp is preferred when both are set. Requires a non-empty pattern after resolve.
func (op *Op) ResolvePatternIR() error {
	if _, err := op.CorePat(); err != nil {
		return err
	}
	return nil
}

// CorePat returns the canonical Core Pat for this op.
// Prefer pattern_sexp when set; otherwise convert PatternIR via NodeToPat.
// Also keeps PatternIR populated (sexpr → Node) for callers that still need Node.
func (op *Op) CorePat() (Pat, error) {
	if len(op.PatternSexp) > 0 && string(op.PatternSexp) != "null" {
		p, err := DecodePatJSON(op.PatternSexp)
		if err != nil {
			return nil, err
		}
		n, err := PatToNode(p)
		if err != nil {
			return nil, fmt.Errorf("pattern_sexp→node: %w", err)
		}
		op.PatternIR = n
		return p, nil
	}
	if op.Pattern != "" {
		p, err := ParseSexp(op.Pattern)
		if err == nil {
			n, nerr := PatToNode(p)
			if nerr == nil {
				op.PatternIR = n
			}
			return p, nil
		}
		return nil, err
	}
	return nil, fmt.Errorf("%w: pattern_sexp or sexp pattern required", ErrRule)
}
