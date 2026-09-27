package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/lewkit/x/text/report"
	"github.com/lewtec/patlint/pkg/apply"
	"github.com/lewtec/patlint/pkg/reporoot"
	"github.com/lewtec/patlint/pkg/script"
	"os"
)

var patlintTool = report.Tool{
	Name:           "patlint",
	InformationURI: "https://github.com/lewtec/patlint",
}

type runCmd struct {
	projectDir
	langFilter
	backupFlag
	format cmd.EnumArg[report.Format] `long:"format" help:"output format: rustc, text, table, or sarif" default:"rustc"`
	fix    cmd.Flag                   `long:"fix" help:"apply non-conflicting fixes from rewrite actions"`
	dryRun cmd.Flag                   `short:"n" long:"dry-run" help:"with --fix: show planned edits without writing"`
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
	// A real --fix rewrites, then scans again. Only that second scan is the report.
	reportLive := !command.fix.Value() || command.dryRun.Value()
	var sink *findingSink
	options.OnFinding = func(finding report.Finding) error {
		if sink == nil {
			return nil
		}
		return sink.Write(finding)
	}
	if reportLive {
		sink = newFindingSink(format, os.Stdout, root)
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
			sink = newFindingSink(format, os.Stdout, root)
			result, err = script.Run(ctx, session, merged, options)
			if err != nil {
				return exitError{exitCode: 2, cause: err}
			}
		}
	}
	if sink == nil {
		sink = newFindingSink(format, os.Stdout, root)
		for _, finding := range result.Findings {
			if err := sink.Write(finding); err != nil {
				return exitError{exitCode: 2, cause: err}
			}
		}
	}
	if err := sink.Close(script.ReportRules(result)); err != nil {
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

// findingSink writes each finding as the scan produces it.
// Text and rustc go out immediately. Table and SARIF are one document, so they flush in Close.
type findingSink struct {
	format report.Format
	w      io.Writer
	root   string
	buf    []report.Finding
	wrote  bool
}

func newFindingSink(format report.Format, w io.Writer, root string) *findingSink {
	return &findingSink{format: format, w: w, root: root}
}

func (s *findingSink) Write(finding report.Finding) error {
	switch s.format {
	case report.FormatText:
		s.wrote = true
		return report.WriteFinding(s.w, finding)
	case report.FormatRustc:
		if s.wrote {
			if _, err := io.WriteString(s.w, "\n"); err != nil {
				return err
			}
		}
		s.wrote = true
		return report.WriteRustc(s.w, s.root, []report.Finding{finding})
	case report.FormatTable, report.FormatSARIF:
		s.buf = append(s.buf, finding)
		return nil
	default:
		return fmt.Errorf("%w %s", report.ErrFormat, s.format.String())
	}
}

func (s *findingSink) Close(rules []report.Rule) error {
	switch s.format {
	case report.FormatTable, report.FormatSARIF:
		return s.format.Render(s.w, s.root, patlintTool, s.buf, rules)
	default:
		return nil
	}
}
