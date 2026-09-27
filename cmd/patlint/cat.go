package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/ui/highlight"
	"github.com/lewtec/patlint/pkg/walker"
)

type catCmd struct {
	projectDir
	color cmd.StringArg `long:"color" help:"when to highlight: auto, always, or never" default:"auto"`
	paths []cmd.StringArg
}

func (catCmd) Description() string {
	return `Print files with structural-tape syntax highlighting.

Color follows the terminal unless --color is set. auto uses color on a
terminal and plain text when stdout is a pipe. never writes the file bytes
unchanged. always emits ANSI colors from the tape token classes.

  patlint cat main.go
  patlint cat --color=always pkg/apply/commit.go`
}

func (command *catCmd) Run(ctx context.Context) error {
	paths := cmd.Values(command.paths)
	if len(paths) == 0 {
		return exitError{exitCode: 2, cause: fmt.Errorf("cat: at least one file is required")}
	}
	useColor, err := catColor(command.color.Value(), os.Stdout)
	if err != nil {
		return exitError{exitCode: 2, cause: err}
	}

	session := newSession(command.directory.Value())
	lispVM, err := pattern.New(prelude.FS)
	if err != nil {
		return exitError{exitCode: 2, cause: err}
	}
	fileWalker, err := walker.NewWalker(ctx, session, lispVM)
	if err != nil {
		return exitError{exitCode: 2, cause: err}
	}

	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return exitError{exitCode: 2, cause: err}
		}
		filePath := path
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(session.Root, filePath)
		}
		source, err := os.ReadFile(filePath)
		if err != nil {
			return exitError{exitCode: 2, cause: err}
		}
		relative := filePath
		if rel, err := filepath.Rel(session.Root, filePath); err == nil && !strings.HasPrefix(rel, "..") {
			relative = rel
		}
		err = highlight.Write(ctx, os.Stdout, source, filepath.ToSlash(relative), highlight.Options{
			Color:  useColor,
			Walker: fileWalker,
		})
		if err != nil {
			return exitError{exitCode: 2, cause: err}
		}
	}
	return nil
}

func catColor(mode string, output *os.File) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		return highlight.AutoColor(output), nil
	case "always":
		return true, nil
	case "never":
		return false, nil
	default:
		return false, fmt.Errorf("cat: --color must be auto, always, or never")
	}
}
