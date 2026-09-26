package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/patlint/pkg/reporoot"
	"github.com/lewtec/patlint/pkg/report"
	"github.com/lewtec/patlint/pkg/script"
	"log/slog"
	"os"
)

type runCmd struct {
	projectDir
	langFilter
	backupFlag
	format cmd.StringArg `long:"format" help:"output format: rustc, text, table, or sarif" default:"rustc"`
	fix    cmd.Flag      `long:"fix" help:"apply non-conflicting fixes from rewrite actions"`
	dryRun cmd.Flag      `short:"n" long:"dry-run" help:"with --fix: show planned edits without writing"`
	packs  []cmd.StringArg
	paths  []cmd.StringArg
}

func (runCmd) Description() string {
	return `Run .rft rule packs over project files.

CLI shape:

  patlint run $where_to_find_rfts -- $files_or_dirs_to_check

Left of "--" is pack roots and/or .rft files. A directory pack loads
non-recursively:

  $dir/*.rft
  $dir/.patlint/*.rft

(lexicographic path order). Right of "--" is the file/dir targets to check.

When the left side is empty, the pack root is the repository root (git pivot
via reporoot.Find from -C). Missing pivot is an error. Zero scripts there
prints a warning and still runs the always-on unused-import builtin.

The dead-imports builtin (rule id imports/unused-named) and prelude
rules_rft.rft (until body scans, callish widen, open lookbehind) are always enabled.

Scripts are top-level forms (no wrapping progn): def, rewrite, rule, builtin.
(rule id level message body) attaches lint metadata for text/SARIF report.

  patlint run -- .
  patlint run .patlint -- pkg/
  patlint run script.rft -- .
  patlint run a.rft b.rft -- ./src

Without "--", all args are pack/script operands and targets default to -C.

Exit 1 if any reported finding remains. Exit 0 when clean.
Tool errors use a non-zero exit with a message on stderr.`
}

func (c *runCmd) Run(ctx context.Context) error {
	format := c.format.Value()
	if _, err := report.NormalizeFormat(format); err != nil {
		return errExit{code: 2, err: err}
	}
	packArgs := cmd.Values(c.packs)
	paths := cmd.Values(c.paths)

	sess := newSession(c.dir.Value())
	root := sess.Root
	if len(paths) == 0 {
		paths = []string{"."}
	}

	scriptPaths, warn, err := resolveRunScripts(packArgs, root)
	if err != nil {
		return errExit{code: 2, err: err}
	}
	if warn != "" {
		fmt.Fprintln(os.Stderr, "warning:", warn)
	}

	var progs []*script.Program
	for _, sp := range scriptPaths {
		prog, err := script.LoadFile(sp)
		if err != nil {
			return errExit{code: 2, err: err}
		}
		progs = append(progs, prog)
	}
	merged := script.MergePrograms(strings.Join(scriptPaths, "+"), progs)
	if merged.Path == "" {
		merged.Path = "<builtin>"
	}
	merged, err = script.EnsureDeadImports(merged)
	if err != nil {
		return errExit{code: 2, err: err}
	}

	opts := script.Options{Paths: paths, LangFilter: c.lang.Value()}
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		opts.OnFinding = func(f report.Finding) {
			_ = report.WriteFinding(os.Stderr, f)
		}
	}

	res, err := script.Run(ctx, sess, merged, opts)
	if err != nil {
		return errExit{code: 2, err: err}
	}

	applyNow := c.fix.Value() && !c.dryRun.Value()
	if c.dryRun.Value() && c.fix.Value() {
		if len(res.ApplyEdits) > 0 {
			if err := applyEditPlan(ctx, os.Stderr, os.Stdin, root, res.ApplyEdits, applyEditPlanOptions{
				DryRun: true,
			}); err != nil {
				return errExit{code: 2, err: err}
			}
		}
	}
	if applyNow && len(res.ApplyEdits) > 0 {
		overlay, err := sessionFromEdits(root, res.ApplyEdits)
		if err != nil {
			return errExit{code: 2, err: err}
		}
		if err := commitOverlay(ctx, os.Stderr, os.Stdin, overlay, applyEditPlanOptions{Backup: c.backup.Value()}); err != nil {
			return errExit{code: 2, err: err}
		}
		res, err = script.Run(ctx, sess, merged, opts)
		if err != nil {
			return errExit{code: 2, err: err}
		}
	}

	if err := report.WriteFormat(ctx, os.Stdout, format, root, res.Findings, script.ReportRules(res)); err != nil {
		return errExit{code: 2, err: err}
	}

	if len(res.Findings) > 0 {
		return errExit{code: 1}
	}
	return nil
}

// resolveRunScripts expands pack operands. Empty packArgs → repo root pack
// (warn if no scripts). Non-empty → ExpandScriptArgs only (no discovery merge).
func resolveRunScripts(packArgs []string, start string) (scripts []string, warn string, err error) {
	if len(packArgs) > 0 {
		scripts, err = script.ExpandScriptArgs(packArgs)
		return scripts, "", err
	}
	packRoot, err := reporoot.Find(start)
	if err != nil {
		return nil, "", fmt.Errorf("run: pack root: %w", err)
	}
	scripts, err = script.ListPackScripts(packRoot)
	if err != nil {
		return nil, "", err
	}
	if len(scripts) == 0 {
		warn = fmt.Sprintf("no .rft scripts under %s (*.rft or %s/*.rft); running builtins only", packRoot, script.PackSubdir)
	}
	return scripts, warn, nil
}
