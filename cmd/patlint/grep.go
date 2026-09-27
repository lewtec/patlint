package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lewtec/lewkit/x/text"
	"github.com/lewtec/patlint/pkg/pattern"
	"os"
)

type grepCmd struct {
	projectDir
	langFilter
	showVariables cmd.Flag      `long:"vars" help:"with --format=text: print captures under each match"`
	format        cmd.StringArg `long:"format" help:"output format: text, csv, or jsonl" default:"text"`
	pattern       cmd.StringArg `help:"pattern"`
	paths         []cmd.StringArg
}

func (grepCmd) Description() string {
	return `Search source files for matches of a structural pattern.

Processing is per-file (map): each file is hop-parsed and matched independently,
and matches are printed as soon as that file is done (streaming; no full-tree
barrier before output).

Patterns are Lisp sexpr (SPEC.md Pattern algebra):
  (token "interface{}")
  (seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) ")")

Matcher spine (composes over Core; findings = non-empty SpanSet):
  (path "**/*_test.go" (token "interface{}"))
  (and (not (path "**/testdata/**")) (token "interface{}"))
  (under (seq (token "func") …) (token "interface{}"))

(ref "provider:path::Symbol") is a hyperlink hole (same targets as the code view).

Output (--format):
  text   (default)  file:line:col: snippet
                    --vars: tab-indented name=value under each match
  csv               header file,line,col,match,<capture…> then one row per hit
                    capture columns are derived statically from the pattern
  jsonl             one JSON object per match (captures map uses the same names)

Formats are pluggable (pattern.GrepFormatter); more can be added later.

Exit status is 0 if any match, 1 if none, 2 on error.`
}

func (command *grepCmd) Run(ctx context.Context) error {
	format := strings.ToLower(strings.TrimSpace(command.format.Value()))
	operation, err := pattern.OpFromCLI("grep", command.language.Value(), command.pattern.Value(), "")
	if err != nil {
		return exitError{exitCode: 2, cause: err}
	}
	session := newSession(command.directory.Value())

	var formatter pattern.GrepFormatter
	switch format {
	case "text":
		formatter = pattern.NewTextGrepFormatter(command.showVariables.Value())
	default:
		formatter, err = pattern.NewGrepFormatter(format)
		if err != nil {
			return exitError{exitCode: 2, cause: err}
		}
	}

	captureNames := pattern.CaptureNames(operation.PatternIR)
	if operation.Matcher != nil {
		captureNames = operation.Matcher.CaptureNames()
	}
	output := os.Stdout
	if err := formatter.Begin(output, captureNames); err != nil {
		return exitError{exitCode: 2, cause: err}
	}

	lispVM, err := pattern.New(prelude.FS)
	if err != nil {
		return exitError{exitCode: 2, cause: err}
	}
	fileWalker, err := walker.NewWalker(ctx, session, lispVM)
	if err != nil {
		return exitError{exitCode: 2, cause: err}
	}

	matchCount := 0
	err = fileWalker.Stream(ctx, operation, pattern.StreamOptions{
		Paths: cmd.Values(command.paths),
		OnMatch: func(match pattern.Match, source []byte) bool {
			line, column, snippet, displayErr := matchDisplay(source, match)
			if displayErr != nil {
				fmt.Fprintf(os.Stderr, "display %s: %v\n", match.File, displayErr)
				return true
			}
			hit := pattern.GrepHit{
				File:     match.File,
				Line:     line,
				Col:      column,
				Match:    snippet,
				Captures: pattern.PublicCaptures(match, source),
			}
			if err := formatter.Format(output, hit, captureNames); err != nil {
				return false
			}
			matchCount++
			return true
		},
	})
	if endErr := formatter.End(output); endErr != nil && err == nil {
		err = endErr
	}
	if err != nil {
		return exitError{exitCode: 2, cause: err}
	}
	if matchCount == 0 {
		return exitError{exitCode: 1}
	}
	return nil
}

func matchDisplay(source []byte, match pattern.Match) (line, column int, snippet string, err error) {
	if int(match.EndByte) > len(source) || match.StartByte > match.EndByte {
		return 0, 0, "", fmt.Errorf("%w in %s", ErrMatchSpan, match.File)
	}
	lineIndex := text.NewLineIndexBytes(source)
	lineNumber, columnIndex := lineIndex.LineColumnAtU32(match.StartByte)
	matchedText := string(source[match.StartByte:match.EndByte])
	if newline := strings.IndexByte(matchedText, '\n'); newline >= 0 {
		matchedText = matchedText[:newline] + "…"
	}
	return lineNumber, columnIndex + 1, matchedText, nil
}

// exitError carries a process exit code for main.
type exitError struct {
	exitCode int
	cause    error
}

func (exit exitError) Error() string {
	if exit.cause == nil {
		return ""
	}
	return exit.cause.Error()
}

func (exit exitError) ExitCode() int { return exit.exitCode }

func (exit exitError) Unwrap() error { return exit.cause }
