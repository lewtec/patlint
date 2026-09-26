package main

import (
	"context"
	"os"

	"github.com/lewtec/lewkit/x/cmd"
)

type cli struct {
	Grep    *grepCmd    `cmd:"grep" help:"Search for structural pattern matches"`
	Rewrite *rewriteCmd `cmd:"rewrite" help:"Rewrite structural pattern matches"`
	RunCmd  *runCmd     `cmd:"run" help:"Run .rft rules (findings, and fixes with --fix)"`
}

func (cli) Description() string {
	return "Lint code with structural patterns"
}

type projectDir struct {
	dir cmd.WorkDirArg `short:"C" long:"dir" help:"project root"`
}

type langFilter struct {
	lang cmd.StringArg `short:"l" long:"lang" help:"language or family filter" default:""`
}

type backupFlag struct {
	backup cmd.Flag `short:"b" long:"backup" help:"create .bak files before writing"`
}

func (cli) Run(context.Context) error {
	return cmd.ErrUsage
}

func Execute(ctx context.Context) error {
	return execute(ctx, os.Args[1:])
}

func execute(ctx context.Context, args []string) error {
	app, err := cmd.Parse[cmd.App[cli]](args...)
	if err != nil {
		return err
	}
	return app.Run(ctx)
}
