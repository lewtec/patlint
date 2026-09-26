// Package ignore collects gitignore-like path patterns and evaluates them
// to decide which project paths should be explored.
//
// Sources (Collect), evaluation order (last match wins):
//  1. built-in dependency/build directory names → dir patterns (node_modules/, …)
//  2. .git/info/exclude when present
//  3. .gitignore files under the root (parent before nested; top-to-bottom in file)
//  4. .gitattributes linguist-generated / refactree-ignored (and -unignore forms)
//
// Path matching uses github.com/git-pkgs/gitignore (git wildmatch). Last match
// wins, including negations. Global core.excludesfile is not loaded.
package ignore

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind identifies where a rule came from.
type Kind string

const (
	KindBuiltinDir        Kind = "builtin-dir"
	KindGitignore         Kind = "gitignore"
	KindLinguistGenerated Kind = "linguist-generated"
	KindRefactreeIgnored  Kind = "refactree-ignored"
)

// Rule is one gitignore-like path pattern.
type Rule struct {
	// Pattern is the path pattern without a leading '!'. Slash-normalized.
	Pattern string
	// Negate is true for un-ignore (!pattern / -linguist-generated / -refactree-ignored).
	Negate bool
	// DirOnly is true when the pattern ends with '/' (directory only).
	DirOnly bool
	// BaseDir is the absolute directory patterns are relative to
	// (project root for builtins; directory of the .gitattributes file otherwise).
	BaseDir string
	// Source is a human provenance string (e.g. "builtin" or path to attributes).
	Source string
	// Line is 1-based in Source when applicable (0 for builtins).
	Line int
	// Kind classifies the rule for debug output.
	Kind Kind
}

// Display returns a gitignore-like rendering of the rule (with ! if negate).
func (r Rule) Display() string {
	p := r.Pattern
	if r.DirOnly && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	if r.Negate {
		return "!" + p
	}
	return p
}

// Provenance is "source:line" or "source" for debug.
func (r Rule) Provenance() string {
	if r.Line > 0 {
		return r.Source + ":" + strconv.Itoa(r.Line)
	}
	return r.Source
}

// String implements fmt.Stringer.
func (r Rule) String() string {
	return fmt.Sprintf("%s  %s  (%s)", r.Display(), r.Provenance(), r.Kind)
}
