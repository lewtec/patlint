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
	showVars cmd.Flag      `long:"vars" help:"with --format=text: print captures under each match"`
	format   cmd.StringArg `long:"format" help:"output format: text, csv, or jsonl" default:"text"`
	pat      cmd.StringArg `help:"pattern"`
	paths    []cmd.StringArg
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

func (c *grepCmd) Run(ctx context.Context) error {
	format := strings.ToLower(strings.TrimSpace(c.format.Value()))
	op, err := pattern.OpFromCLI("grep", c.lang.Value(), c.pat.Value(), "")
	if err != nil {
		return errExit{code: 2, err: err}
	}
	sess := newSession(c.dir.Value())

	var enc pattern.GrepFormatter
	switch format {
	case "text":
		enc = pattern.NewTextGrepFormatter(c.showVars.Value())
	default:
		enc, err = pattern.NewGrepFormatter(format)
		if err != nil {
			return errExit{code: 2, err: err}
		}
	}

	varNames := pattern.CaptureNames(op.PatternIR)
	if op.Matcher != nil {
		varNames = op.Matcher.CaptureNames()
	}
	out := os.Stdout
	if err := enc.Begin(out, varNames); err != nil {
		return errExit{code: 2, err: err}
	}

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		return errExit{code: 2, err: err}
	}
	w, err := walker.NewWalker(sess, vm)
	if err != nil {
		return errExit{code: 2, err: err}
	}

	matchCount := 0
	err = w.Stream(ctx, op, pattern.StreamOptions{
		Paths: cmd.Values(c.paths),
		OnMatch: func(m pattern.Match, source []byte) bool {
			line, col, snippet, err := matchDisplay(source, m)
			if err != nil {
				fmt.Fprintf(os.Stderr, "display %s: %v\n", m.File, err)
				return true
			}
			hit := pattern.GrepHit{
				File:     m.File,
				Line:     line,
				Col:      col,
				Match:    snippet,
				Captures: pattern.PublicCaptures(m, source),
			}
			if err := enc.Format(out, hit, varNames); err != nil {
				return false
			}
			matchCount++
			return true
		},
	})
	if endErr := enc.End(out); endErr != nil && err == nil {
		err = endErr
	}
	if err != nil {
		return errExit{code: 2, err: err}
	}
	if matchCount == 0 {
		return errExit{code: 1}
	}
	return nil
}

func matchDisplay(src []byte, m pattern.Match) (line, col int, snippet string, err error) {
	if int(m.EndByte) > len(src) || m.StartByte > m.EndByte {
		return 0, 0, "", fmt.Errorf("%w in %s", ErrMatchSpan, m.File)
	}
	li := text.NewLineIndexBytes(src)
	l, c0 := li.LineColumnAtU32(m.StartByte)
	text := string(src[m.StartByte:m.EndByte])
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i] + "…"
	}
	return l, c0 + 1, text, nil
}

// errExit carries a process exit code for main.
type errExit struct {
	code int
	err  error
}

func (e errExit) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e errExit) ExitCode() int { return e.code }

func (e errExit) Unwrap() error { return e.err }
