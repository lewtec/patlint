package walker

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/projectfs"
	"github.com/lewtec/patlint/pkg/store"
)

// Run matches op on this Walker (optional path filter).
func (w *Walker) Run(ctx context.Context, op pattern.Op, opts pattern.RunOptions) (pattern.RunResult, error) {
	var out pattern.RunResult
	err := w.Stream(ctx, op, pattern.StreamOptions{
		Paths: opts.Paths,
		OnMatch: func(m pattern.Match, _ []byte) bool {
			out.Matches = append(out.Matches, m)
			return true
		},
		OnFile: func(rel string, matches []pattern.Match, fileEdits []project.Edit, _ []byte) bool {
			if len(fileEdits) > 0 {
				out.Edits = append(out.Edits, fileEdits...)
			}
			return true
		},
	})
	return out, err
}

// WalkSourceFiles streams FileExtract values with this Walker (VM host + extract).
func (w *Walker) WalkSourceFiles(ctx context.Context, paths []string, fsys projectfs.FS, fn func(*project.FileExtract) error) error {
	if w == nil || w.Sess == nil {
		return ingest.ErrNilSession
	}
	if w.VM == nil {
		return fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	rootAbs := w.Sess.Root
	if rootAbs == "" {
		var err error
		rootAbs, err = filepath.Abs(".")
		if err != nil {
			return err
		}
	}
	return walkExtractSources(ctx, w, rootAbs, paths, fsys, fn)
}

// WalkStore streams file paths after ingesting each file into st.
func (w *Walker) WalkStore(ctx context.Context, paths []string, fsys projectfs.FS, st *store.Store, fn func(path string) error) error {
	if w == nil || w.Sess == nil {
		return ingest.ErrNilSession
	}
	if w.VM == nil {
		return fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	if st == nil {
		return fmt.Errorf("WalkStore: nil store")
	}
	rootAbs := w.Sess.Root
	if rootAbs == "" {
		var err error
		rootAbs, err = filepath.Abs(".")
		if err != nil {
			return err
		}
	}
	return walkStoreSources(ctx, w, rootAbs, paths, fsys, st, fn)
}

// Stream matches op on this Walker view.
func (w *Walker) Stream(ctx context.Context, op pattern.Op, opts pattern.StreamOptions) error {
	if w == nil || w.Sess == nil {
		return ingest.ErrNilSession
	}
	return pattern.Stream(ctx, w.Sess, w.VM, op, opts)
}

// Apply runs a rewrite op and writes edits under Walker.Sess.Root, file-by-file.
func (w *Walker) Apply(ctx context.Context, op pattern.Op, opts pattern.RunOptions) (pattern.RunResult, error) {
	if w == nil || w.Sess == nil {
		return pattern.RunResult{}, ingest.ErrNilSession
	}
	if op.Mode != "rewrite" {
		return pattern.RunResult{}, fmt.Errorf("%w: Apply: mode is %q, want rewrite", pattern.ErrRule, op.Mode)
	}
	root := w.Sess.Root
	var out pattern.RunResult
	var applyErr error
	err := w.Stream(ctx, op, pattern.StreamOptions{
		Paths: opts.Paths,
		OnMatch: func(m pattern.Match, _ []byte) bool {
			out.Matches = append(out.Matches, m)
			return true
		},
		OnFile: func(rel string, matches []pattern.Match, fileEdits []project.Edit, _ []byte) bool {
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

func walkExtractSources(ctx context.Context, w *Walker, rootAbs string, paths []string, fsys projectfs.FS, fn func(*project.FileExtract) error) error {
	if w == nil {
		return ingest.ErrNilSession
	}
	statFS := fsys
	if statFS == nil {
		statFS = projectfs.OS{}
	}
	run := func(src ingest.ExtractSource) error {
		var walkErr error
		err := w.WalkExtracts(ctx, src, func(fe *project.FileExtract) bool {
			if walkErr != nil {
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

func walkStoreSources(ctx context.Context, w *Walker, rootAbs string, paths []string, fsys projectfs.FS, st *store.Store, fn func(path string) error) error {
	if w == nil {
		return ingest.ErrNilSession
	}
	statFS := fsys
	if statFS == nil {
		statFS = projectfs.OS{}
	}
	run := func(src ingest.ExtractSource) error {
		src.Session = w.Sess
		src.Policy = w.VM
		var walkErr error
		err := ingest.WalkStore(ctx, src, st, func(path string) bool {
			if walkErr != nil {
				return false
			}
			if err := fn(path); err != nil {
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
		info, err := statFS.Stat(abs)
		if err != nil {
			return err
		}
		if info.IsDir() {
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
