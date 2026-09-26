package main

import (
	"context"

	"os"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/apply"
	"github.com/lewtec/patlint/pkg/pattern"
)

type rewriteCmd struct {
	projectDir
	langFilter
	backupFlag
	interactive cmd.Flag      `short:"i" long:"interactive" help:"show edit plan and ask for confirmation"`
	dryRun      cmd.Flag      `short:"n" long:"dry-run" help:"show edit plan without writing"`
	pattern     cmd.StringArg `help:"pattern"`
	replacement cmd.StringArg `help:"replacement"`
	paths       []cmd.StringArg
}

func (rewriteCmd) Description() string {
	return `Find matches of a structural pattern and replace with a sexp emit.

Same as (rewrite MATCH EMIT) in a .rft script. Processing is per-file (map).

Emit (SPEC.md Pattern algebra §16.6):
  any                      literal text (one sexp atom)
  "return max(x, y)"       quoted literal (spaces)
  (slot)                   original text of the edit span
  (ref "provider:path::N") product selector (+ import ensure)
  (seq EMIT…)              concatenation

Locus is the match root, or (take "name" MATCH) for that capture's spans.

Examples:
  patlint rewrite '(token "interface{}")' 'any'
  patlint rewrite \
    '(take "MSG" (seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG (regex "(?i)^failed to\\s+(.*)" 1)) "," (capture ERR any) ")"))' \
    '(seq "\"" (slot) "\"")'`
}

func (c *rewriteCmd) Run(ctx context.Context) error {
	patternText, replacement := c.pattern.Value(), c.replacement.Value()
	language := c.lang.Value()
	if _, err := pattern.OpFromCLI("rewrite", language, patternText, replacement); err != nil {
		return err
	}
	session := newSession(c.dir.Value())
	source := "(rewrite " + patternText + " " + replacement + ")"
	if language != "" {
		source = "(rewrite (under (lang " + language + ") " + patternText + ") " + replacement + ")"
	}
	vm, err := pattern.New(prelude.FS, pattern.FromString("rewrite.rft", source))
	if err != nil {
		return err
	}
	out, err := vm.Run(ctx, session, cmd.Values(c.paths)...)
	if err != nil {
		return err
	}
	committer := apply.Committer{
		StandardError: os.Stderr,
		Input:         os.Stdin,
		Interactive:   c.interactive.Value(),
		DryRun:        c.dryRun.Value(),
		Backup:        c.backup.Value(),
	}
	return committer.Session(ctx, out)
}
