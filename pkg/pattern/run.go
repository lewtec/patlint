package pattern

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingestutil"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/projectfs"
	"github.com/lewtec/patlint/pkg/store"
)

// RunResult is the collected outcome of a run (non-streaming convenience).
type RunResult struct {
	Matches []Match
	Edits   []project.Edit
}

// RunOptions controls which files are scanned under Root.
type RunOptions struct {
	// Paths are optional file or directory paths (absolute or relative to Root).
	// Empty means walk the whole Root (ExtractDir).
	Paths []string
}

// StreamOptions is RunOptions plus per-match / per-file callbacks for map-style streaming.
type StreamOptions struct {
	Paths []string

	// OnMatch is invoked for each match as soon as its file is processed.
	// source is that file's bytes (for Span.Text). Return false to stop early.
	OnMatch func(m Match, source []byte) bool

	// OnFile is invoked after a file is matched (and, for rewrite, after its edits
	// are computed). source is that file's bytes. Return false to stop.
	OnFile func(rel string, matches []Match, fileEdits []project.Edit, source []byte) bool
}

// OpFromCLI builds an Op from grep/rewrite argv (sexp pattern).
// Patterns go through the matcher spine (path/under/and/or/not + Core leaf).
func OpFromCLI(mode, lang, patternStr, replacementStr string) (Op, error) {
	mexpr, err := ParseMatcher(patternStr)
	if err != nil {
		return Op{}, fmt.Errorf("pattern: %w", err)
	}
	cm, err := CompileMatcher(mexpr)
	if err != nil {
		return Op{}, fmt.Errorf("pattern: %w", err)
	}
	node, err := matcherPatternNode(cm)
	if err != nil {
		return Op{}, fmt.Errorf("pattern: %w", err)
	}
	op := Op{
		Mode:      mode,
		Lang:      lang,
		Pattern:   patternStr,
		PatternIR: node,
		Matcher:   cm,
	}
	if mode == "rewrite" {
		emit, err := ParseEmit(replacementStr)
		if err != nil {
			return Op{}, fmt.Errorf("emit: %w", err)
		}
		repl := replacementStr
		op.Replacement = &repl
		op.Emit = emit
		op.Take = matcherTakeName(mexpr)
	}
	return op, nil
}

// matcherPatternNode is the Node used for capture-name / import helpers.
func matcherPatternNode(cm *CompiledMatcher) (Node, error) {
	if leaf, ok := cm.LeafPat(); ok {
		return PatToNode(leaf)
	}
	// Composite matcher: placeholder (matches come from Matcher, not this Node).
	return Node{Kind: "seq", As: "ROOT"}, nil
}

const maxGrepFileBytes = 256 << 10 // 256 KiB

// Stream matches op on sess+vm (LispVM.Run and Walker.Stream).
func Stream(ctx context.Context, sess *project.Session, vm *LispVM, op Op, opts StreamOptions) error {
	if sess == nil {
		return ingest.ErrNilSession
	}
	if vm == nil {
		return fmt.Errorf("%w: walker: nil LispVM", ErrExtract)
	}
	cm := op.Matcher
	var core Pat
	var err error
	if cm == nil {
		core, err = op.CorePat()
		if err != nil {
			return fmt.Errorf("pattern: %w", err)
		}
	}
	var rule Rule
	if op.Mode == "rewrite" {
		rule, err = RuleFromOp(op)
		if err != nil {
			return err
		}
	}

	rootAbs := sess.Root
	if rootAbs == "" {
		rootAbs, err = filepath.Abs(".")
		if err != nil {
			return err
		}
	}

	needLinks := false
	if cm != nil {
		needLinks = cm.NeedsLinks()
	} else {
		needLinks = PatNeedsLinks(core)
	}
	stop := false
	view := sess.FS
	if view == nil {
		view = os.DirFS(rootAbs)
	}

	return walkExtractSources(ctx, sess, vm, rootAbs, opts.Paths, nil, func(fe *project.FileExtract) error {
		if stop {
			return nil
		}
		if fe == nil {
			return nil
		}
		if op.Lang != "" && fe.Language != op.Lang {
			return nil
		}

		rel := strings.TrimPrefix(filepath.ToSlash(fe.Path), "./")
		if cm != nil && !cm.AcceptsFile(rel) {
			return nil
		}
		abs := lewpath.New(rootAbs, filepath.FromSlash(rel)).String()

		if st, err := fs.Stat(view, rel); err == nil && st.Size() > maxGrepFileBytes {
			if !isExplicitFilePath(rootAbs, opts.Paths, abs) {
				slog.Debug("pattern skip large file", "path", rel, "size", st.Size())
				return nil
			}
		}

		slog.Debug("pattern visit", "path", rel, "lang", fe.Language)

		source, err := fs.ReadFile(view, rel)
		if err != nil {
			return err
		}
		pf, err := ingestutil.ParseSource(ctx, sess.Engine(), source, rel, vm.GrammarForLanguage(fe.Language))
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}

		var fileResult *project.Result
		if needLinks {
			st := store.New()
			ingest.Ingest(st, []*project.FileExtract{fe}, vm)
			var evalErr error
			fileResult, evalErr = ingest.EvalStore(ctx, rootAbs, st, vm)
			if evalErr != nil {
				return evalErr
			}
		}

		var ms []Match
		pol := vm.TapePolicy(rel)
		scratch := &fileScratch{pol: pol, forLang: vm.tapePolicyForLang}
		if cm != nil {
			scratch.want = nodeTypesFromFinder(cm.root)
			ms, err = matchFileMatcherPol(ctx, sess, rootAbs, rel, source, pf.Root, cm, fileResult, pol, scratch)
		} else {
			ms, err = matchFilePatPol(sess, rootAbs, rel, source, pf.Root, core, fileResult, pol)
		}
		pf.Close()
		if err != nil {
			return fmt.Errorf("match %s: %w", rel, err)
		}

		for _, m := range ms {
			if opts.OnMatch != nil && !opts.OnMatch(m, source) {
				stop = true
				return nil
			}
		}

		var fileEdits []project.Edit
		if op.Mode == "rewrite" && len(ms) > 0 {
			fileEdits, err = rule.Edits(ms, source)
			if err != nil {
				return err
			}
			if len(fileEdits) > 0 {
				needs := ImportNeedsForRule(vm, fe.Language, rule)
				prune := ingest.PruneImportOpts{MaskSpans: SiteEditMaskSpans(fileEdits)}
				fileEdits = WithImportHygiene(vm, rel, fe.Language, source, fe, fileEdits, needs, prune)
			}
		}
		if opts.OnFile != nil && !opts.OnFile(rel, ms, fileEdits, source) {
			stop = true
			return nil
		}
		return nil
	})
}

func walkExtractSources(ctx context.Context, sess *project.Session, vm *LispVM, rootAbs string, paths []string, fsys projectfs.FS, fn func(*project.FileExtract) error) error {
	if sess == nil {
		return ingest.ErrNilSession
	}
	statFS := fsys
	if statFS == nil {
		statFS = projectfs.OS{}
	}
	run := func(src ingest.ExtractSource) error {
		src.Session = sess
		src.Policy = vm
		var walkErr error
		err := ingest.WalkExtracts(ctx, src, func(fe *project.FileExtract) bool {
			if walkErr != nil {
				return false
			}
			if err := ctx.Err(); err != nil {
				walkErr = err
				return false
			}
			if err := fn(fe); err != nil {
				walkErr = err
				return false
			}
			return true
		})
		if err != nil {
			return err
		}
		return walkErr
	}
	if len(paths) == 0 {
		return run(ingest.ExtractSource{
			Kind:      ingest.ExtractDir,
			Root:      rootAbs,
			Recursive: true,
			FS:        fsys,
		})
	}

	var filePaths []string
	var dirPaths []string
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = lewpath.New(rootAbs, p).String()
		}
		abs, err := filepath.Abs(abs)
		if err != nil {
			return err
		}
		st, err := statFS.Stat(abs)
		if err != nil {
			return err
		}
		if st.IsDir() {
			dirPaths = append(dirPaths, abs)
		} else {
			filePaths = append(filePaths, abs)
		}
	}

	if len(filePaths) > 0 {
		if err := run(ingest.ExtractSource{
			Kind:  ingest.ExtractHop,
			Root:  rootAbs,
			Paths: filePaths,
			FS:    fsys,
		}); err != nil {
			return err
		}
	}
	for _, dirAbs := range dirPaths {
		relDir := dirAbs
		if r, err := filepath.Rel(rootAbs, dirAbs); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			relDir = r
		}
		if err := run(ingest.ExtractSource{
			Kind:      ingest.ExtractDir,
			Root:      rootAbs,
			Dir:       relDir,
			Recursive: true,
			FS:        fsys,
		}); err != nil {
			return err
		}
	}
	return nil
}

func runOp(ctx context.Context, sess *project.Session, vm *LispVM, op Op, opts RunOptions) (RunResult, error) {
	var out RunResult
	err := Stream(ctx, sess, vm, op, StreamOptions{
		Paths: opts.Paths,
		OnMatch: func(m Match, _ []byte) bool {
			out.Matches = append(out.Matches, m)
			return true
		},
		OnFile: func(_ string, _ []Match, fileEdits []project.Edit, _ []byte) bool {
			if len(fileEdits) > 0 {
				out.Edits = append(out.Edits, fileEdits...)
			}
			return true
		},
	})
	return out, err
}

func applyOp(ctx context.Context, sess *project.Session, vm *LispVM, op Op, opts RunOptions) (RunResult, error) {
	if op.Mode != "rewrite" {
		return RunResult{}, fmt.Errorf("%w: Apply: mode is %q, want rewrite", ErrRule, op.Mode)
	}
	root := sess.Root
	var out RunResult
	var applyErr error
	err := Stream(ctx, sess, vm, op, StreamOptions{
		Paths: opts.Paths,
		OnMatch: func(m Match, _ []byte) bool {
			out.Matches = append(out.Matches, m)
			return true
		},
		OnFile: func(_ string, _ []Match, fileEdits []project.Edit, _ []byte) bool {
			if len(fileEdits) == 0 {
				return true
			}
			out.Edits = append(out.Edits, fileEdits...)
			if err := project.ApplyEdits(ctx, root, fileEdits); err != nil {
				applyErr = err
				return false
			}
			return true
		},
	})
	if err != nil {
		return out, err
	}
	return out, applyErr
}

func isExplicitFilePath(rootAbs string, paths []string, abs string) bool {
	for _, p := range paths {
		cand := p
		if !filepath.IsAbs(cand) {
			cand = lewpath.New(rootAbs, p).String()
		}
		cand, err := filepath.Abs(cand)
		if err != nil {
			continue
		}
		if cand == abs {
			if st, err := (projectfs.OS{}).Stat(cand); err == nil && !st.IsDir() {
				return true
			}
		}
	}
	return false
}
