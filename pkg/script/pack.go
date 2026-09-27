package script

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/lewkit/x/text/report"
	"github.com/lewtec/patlint/pkg/projectfs"
)

// PackSubdir is the conventional directory for project .rft rules under a pack root.
const PackSubdir = ".patlint"

// BuiltinDeadImports is the engine hook name for unused named import prune.
const BuiltinDeadImports = "dead-imports"

// DeadImportsRuleID is the always-on reporting id for dead-imports.
const DeadImportsRuleID = "imports/unused-named"

// ListPackScripts returns absolute paths of .rft scripts for a pack root, non-recursive:
//
//	$dir/*.rft  and  $dir/.patlint/*.rft
//
// Sorted lexicographically by full path for deterministic merge order.
func ListPackScripts(dir string) ([]string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	st, err := (projectfs.OS{}).Stat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("pack root is not a directory: %s", abs)
	}

	var out []string
	for _, sub := range []string{abs, lewpath.New(abs, PackSubdir).String()} {
		ents, err := (projectfs.OS{}).ReadDir(sub)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".rft") {
				continue
			}
			out = append(out, lewpath.New(sub, name).String())
		}
	}
	slices.Sort(out)
	return out, nil
}

// ExpandScriptArgs turns CLI pack operands into absolute .rft paths.
// Each arg is either a .rft file or a pack directory (ListPackScripts).
// An explicit directory with zero scripts is an error.
func ExpandScriptArgs(args []string) ([]string, error) {
	var out []string
	for _, a := range args {
		abs, err := filepath.Abs(a)
		if err != nil {
			return nil, err
		}
		st, err := (projectfs.OS{}).Stat(abs)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			if !strings.HasSuffix(abs, ".rft") {
				return nil, fmt.Errorf("not a .rft script or pack directory: %s", abs)
			}
			out = append(out, abs)
			continue
		}
		scripts, err := ListPackScripts(abs)
		if err != nil {
			return nil, err
		}
		if len(scripts) == 0 {
			return nil, fmt.Errorf("pack directory has no *.rft (or %s/*.rft): %s", PackSubdir, abs)
		}
		out = append(out, scripts...)
	}
	return out, nil
}

// DeadImportsAction is the always-on unused-named-import builtin.
func DeadImportsAction() Action {
	return Action{
		Report: &Report{
			ID:      DeadImportsRuleID,
			Level:   report.LevelWarning,
			Message: "Unused named import",
		},
		Builtin: BuiltinDeadImports,
		Source:  "<builtin>",
		Index:   -1,
	}
}

// EnsureDeadImports prepends the always-on dead-imports action when the program
// does not already include that builtin, then merges prelude rules_rft.rft
// rules. Recompiles the spine plan.
func EnsureDeadImports(prog *Program) (*Program, error) {
	if prog == nil {
		prog = &Program{Path: "<builtin>"}
	}
	hasDead := false
	for _, a := range prog.Actions {
		if a.Builtin == BuiltinDeadImports {
			hasDead = true
			break
		}
	}
	if !hasDead {
		prog.Actions = append([]Action{DeadImportsAction()}, prog.Actions...)
	}
	return mergePreludeRules(prog)
}

// MergePrograms concatenates actions from progs in order into one Program.
func MergePrograms(path string, progs []*Program) *Program {
	merged := &Program{Path: path}
	for _, p := range progs {
		if p == nil {
			continue
		}
		merged.Actions = append(merged.Actions, p.Actions...)
		merged.packSrcs = append(merged.packSrcs, p.packSrcs...)
	}
	merged.plan = CompilePlan(merged)
	return merged
}
