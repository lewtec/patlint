package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/patlint/pkg/apply"
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

func (command *runCmd) Run(ctx context.Context) error {
	format := command.format.Value()
	if _, err := report.NormalizeFormat(format); err != nil {
		return exitError{exitCode: 2, cause: err}
	}
	packArguments := cmd.Values(command.packs)
	paths := cmd.Values(command.paths)

	session := newSession(command.directory.Value())
	root := session.Root
	if len(paths) == 0 {
		paths = []string{"."}
	}

	scriptPaths, warning, err := resolveRunScripts(packArguments, root)
	if err != nil {
		return exitError{exitCode: 2, cause: err}
	}
	if warning != "" {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}

	var programs []*script.Program
	for _, scriptPath := range scriptPaths {
		program, err := script.LoadFile(scriptPath)
		if err != nil {
			return exitError{exitCode: 2, cause: err}
		}
		programs = append(programs, program)
	}
	merged := script.MergePrograms(strings.Join(scriptPaths, "+"), programs)
	if merged.Path == "" {
		merged.Path = "<builtin>"
	}
	merged, err = script.EnsureDeadImports(merged)
	if err != nil {
		return exitError{exitCode: 2, cause: err}
	}

	options := script.Options{Paths: paths, LangFilter: command.language.Value()}
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		options.OnFinding = func(finding report.Finding) {
			_ = report.WriteFinding(os.Stderr, finding)
		}
	}

	result, err := script.Run(ctx, session, merged, options)
	if err != nil {
		return exitError{exitCode: 2, cause: err}
	}

	committer := apply.Committer{
		StandardError: os.Stderr,
		Input:         os.Stdin,
		DryRun:        command.dryRun.Value(),
		Backup:        command.backup.Value(),
	}
	if command.fix.Value() && len(result.ApplyEdits) > 0 {
		if err := committer.Edits(ctx, root, result.ApplyEdits); err != nil {
			return exitError{exitCode: 2, cause: err}
		}
		if !committer.DryRun {
			result, err = script.Run(ctx, session, merged, options)
			if err != nil {
				return exitError{exitCode: 2, cause: err}
			}
		}
	}

	if err := report.WriteFormat(ctx, os.Stdout, format, root, result.Findings, script.ReportRules(result)); err != nil {
		return exitError{exitCode: 2, cause: err}
	}

	if len(result.Findings) > 0 {
		return exitError{exitCode: 1}
	}
	return nil
}

// resolveRunScripts expands pack operands. Empty packArguments → repo root pack
// (warn if no scripts). Non-empty → ExpandScriptArgs only (no discovery merge).
func resolveRunScripts(packArguments []string, startDirectory string) (scripts []string, warning string, err error) {
	if len(packArguments) > 0 {
		scripts, err = script.ExpandScriptArgs(packArguments)
		return scripts, "", err
	}
	packRoot, err := reporoot.Find(startDirectory)
	if err != nil {
		return nil, "", fmt.Errorf("run: pack root: %w", err)
	}
	scripts, err = script.ListPackScripts(packRoot)
	if err != nil {
		return nil, "", err
	}
	if len(scripts) == 0 {
		warning = fmt.Sprintf("no .rft scripts under %s (*.rft or %s/*.rft); running builtins only", packRoot, script.PackSubdir)
	}
	return scripts, warning, nil
}
