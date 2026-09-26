package ignore

import (
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/git-pkgs/gitignore"
)

// Engine evaluates collected gitignore-like rules for a project root.
// Matching is a *gitignore.Matcher; Rules are the ordered list we loaded into it.
type Engine struct {
	Root string
	// Rules in evaluation order (last match wins). Debug listing + SkipDir.
	Rules []Rule
	// Sources lists absolute .gitignore / .gitattributes paths that contributed rules.
	Sources []string

	m *gitignore.Matcher
}

// Decision is the result of evaluating a path.
type Decision struct {
	// Explore is true when the path should be crawled/scanned.
	Explore bool
	// Pattern is the winning pattern text when a rule matched (debug).
	Pattern string
	// Source is the pattern origin path when known (debug).
	Source string
	// Line is 1-based in Source when known (debug).
	Line int
	// Kind is the winning rule class (empty if no rule matched).
	Kind Kind
}

// Explore reports whether path should be included in a project walk/scan.
func (e *Engine) Explore(path string) bool {
	return e.Check(path).Explore
}

// Check evaluates path (file or directory) against all rules.
func (e *Engine) Check(path string) Decision {
	if e == nil {
		return Decision{Explore: true}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	isDir := false
	if st, err := os.Stat(abs); err == nil {
		isDir = st.IsDir()
	}
	return e.decide(abs, isDir)
}

// CheckPath evaluates path with an explicit isDir flag (no Stat).
func (e *Engine) CheckPath(path string, isDir bool) Decision {
	if e == nil {
		return Decision{Explore: true}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	return e.decide(abs, isDir)
}

func (e *Engine) decide(abs string, isDir bool) Decision {
	if e == nil || e.m == nil {
		return Decision{Explore: true}
	}
	rel, err := filepath.Rel(e.Root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Decision{Explore: true}
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" {
		return Decision{Explore: true}
	}
	if isDir {
		rel += "/"
	}
	hit := e.m.MatchDetail(rel)
	if !hit.Matched {
		return Decision{Explore: true}
	}
	return Decision{
		Explore: !hit.Ignored,
		Pattern: hit.Pattern,
		Source:  hit.Source,
		Line:    hit.Line,
		Kind:    e.kindOf(hit.Pattern),
	}
}

func (e *Engine) kindOf(display string) Kind {
	for i := len(e.Rules) - 1; i >= 0; i-- {
		if e.Rules[i].Display() == display {
			return e.Rules[i].Kind
		}
	}
	return ""
}

// SkipDir reports whether a product crawl should not enter this directory
// (gitignore, builtins, linguist-generated, refactree-ignored).
// If the directory is ignored but a later negation could apply under it,
// returns false so the walk can still find un-ignored children.
func (e *Engine) SkipDir(path string) bool {
	if e == nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	if e.decide(abs, true).Explore {
		return false
	}
	return !e.hasNegateUnder(abs)
}

// GitSkipDir reports whether a VCS walk should not enter this directory:
// builtin skip names and .gitignore / exclude. linguist-generated and
// refactree-ignored do not skip — those are product-crawl marks only.
func (e *Engine) GitSkipDir(path string) bool {
	return e.GitHidden(path, true)
}

// GitHidden is the VCS half of ignore: gitignore, exclude, and builtin
// skip dirs. Product marks (refactree-ignored, linguist-generated) are not hidden.
func (e *Engine) GitHidden(path string, isDir bool) bool {
	if isDir && IsSkippedDirName(filepath.Base(path)) {
		return true
	}
	if e == nil {
		return false
	}
	d := e.CheckPath(path, isDir)
	if d.Explore {
		return false
	}
	switch d.Kind {
	case KindRefactreeIgnored, KindLinguistGenerated:
		return false
	default:
		return true
	}
}

func (e *Engine) hasNegateUnder(dirAbs string) bool {
	for i := range e.Rules {
		r := &e.Rules[i]
		if !r.Negate {
			continue
		}
		if rel, err := filepath.Rel(dirAbs, r.BaseDir); err == nil && (rel == "." || !strings.HasPrefix(rel, "..")) {
			return true
		}
		if strings.ContainsAny(r.Pattern, "*?[") {
			if rel, err := filepath.Rel(r.BaseDir, dirAbs); err == nil && (rel == "." || !strings.HasPrefix(rel, "..")) {
				return true
			}
			continue
		}
		cand := lewpath.New(r.BaseDir, filepath.FromSlash(r.Pattern)).String()
		if rel, err := filepath.Rel(dirAbs, cand); err == nil && (rel == "." || !strings.HasPrefix(rel, "..")) {
			return true
		}
	}
	return false
}
