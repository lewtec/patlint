package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/lewtec/lewkit/x/cmd"
	lewfs "github.com/lewtec/lewkit/x/fs"
	"github.com/lewtec/lewkit/x/fs/zip"
	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
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

Files are read through lewkit x/fs. A .zip argument is opened with x/fs/zip
and each member is printed.

Color follows the terminal unless --color is set. auto uses color on a
terminal and plain text when stdout is a pipe. never writes the file bytes
unchanged. always emits ANSI colors from the tape token classes.

  patlint cat main.go
  patlint cat --color=always pkg/apply/commit.go
  patlint cat src.zip`
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

	for _, argument := range paths {
		if err := ctx.Err(); err != nil {
			return exitError{exitCode: 2, cause: err}
		}
		if err := catPath(ctx, session, fileWalker, argument, useColor); err != nil {
			return exitError{exitCode: 2, cause: err}
		}
	}
	return nil
}

func catPath(ctx context.Context, session *project.Session, fileWalker *walker.Walker, argument string, useColor bool) error {
	relative, err := catRelative(session.Root, argument)
	if err != nil {
		return err
	}
	if archiveKind(relative) != "" {
		return catArchive(ctx, session, fileWalker, relative, useColor)
	}
	source, err := readFile(ctx, session.FS, relative)
	if err != nil {
		return err
	}
	return highlight.Write(ctx, os.Stdout, source, relative, highlight.Options{
		Color:  useColor,
		Walker: fileWalker,
	})
}

func catArchive(ctx context.Context, session *project.Session, fileWalker *walker.Walker, relative string, useColor bool) error {
	file, err := session.FS.Open(relative)
	if err != nil {
		return err
	}
	defer file.Close()
	filesystem, err := openArchive(ctx, file, relative)
	if err != nil {
		return err
	}
	return fs.WalkDir(filesystem, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		source, err := fs.ReadFile(filesystem, name)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(os.Stdout, "# %s\n", path.Join(relative, name)); err != nil {
			return err
		}
		return highlight.Write(ctx, os.Stdout, source, name, highlight.Options{
			Color:  useColor,
			Walker: fileWalker,
		})
	})
}

func openArchive(ctx context.Context, reader io.Reader, name string) (fs.FS, error) {
	switch archiveKind(name) {
	case "zip":
		return zip.Open(ctx, reader)
	default:
		return nil, fmt.Errorf("cat: %s is not an archive lewkit x/fs can open", name)
	}
}

// readFile indexes one project file into an x/fs tree and reads it back.
func readFile(ctx context.Context, filesystem fs.FS, name string) ([]byte, error) {
	opened, err := filesystem.Open(name)
	if err != nil {
		return nil, err
	}
	defer opened.Close()
	info, err := opened.Stat()
	if err != nil {
		return nil, err
	}
	tree, err := lewfs.New(ctx, func(yield func(lewfs.File, error) bool) {
		yield(lewfs.File{
			Name:    lewpath.New(name),
			Mode:    info.Mode(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Reader:  opened,
		}, nil)
	})
	if err != nil {
		return nil, err
	}
	return tree.ReadFile(name)
}

func archiveKind(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return ""
	}
}

func catRelative(root, argument string) (string, error) {
	filePath := argument
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(root, filePath)
	}
	relative, err := filepath.Rel(root, filePath)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cat: %s is outside the project root", argument)
	}
	name := filepath.ToSlash(relative)
	if err := lewfs.CheckName("open", name); err != nil {
		return "", err
	}
	return name, nil
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
