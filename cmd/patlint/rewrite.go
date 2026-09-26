package main

import (
	"context"

	"github.com/lewtec/patlint/internal/prelude"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/patlint/pkg/pattern"
	"os"
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
	pat, repl := c.pattern.Value(), c.replacement.Value()
	lang := c.lang.Value()
	if _, err := pattern.OpFromCLI("rewrite", lang, pat, repl); err != nil {
		return err
	}
	sess := newSession(c.dir.Value())
	src := "(rewrite " + pat + " " + repl + ")"
	if lang != "" {
		src = "(rewrite (under (lang " + lang + ") " + pat + ") " + repl + ")"
	}
	vm, err := pattern.New(prelude.FS, pattern.FromString("rewrite.rft", src))
	if err != nil {
		return err
	}
	out, err := vm.Run(ctx, sess, cmd.Values(c.paths)...)
	if err != nil {
		return err
	}
	return commitOverlay(ctx, os.Stderr, os.Stdin, out, applyEditPlanOptions{
		Interactive: c.interactive.Value(),
		DryRun:      c.dryRun.Value(),
		Backup:      c.backup.Value(),
	})
}
