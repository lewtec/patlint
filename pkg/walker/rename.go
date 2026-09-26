package walker

import (
	"context"
	"fmt"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/projectfs"
)

// Rename loads the closed project graph into the store (ExpandImports) and plans.
func (w *Walker) Rename(ctx context.Context, dir, sourceRef, destRef string) (project.Plan, error) {
	if w == nil || w.Sess == nil {
		return project.Plan{}, ingest.ErrNilSession
	}
	if w.VM == nil {
		return project.Plan{}, fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	st, err := w.LoadStore(ctx, ingest.SourceProject(dir), ingest.MaterializeOptions{ExpandImports: true})
	if err != nil {
		return project.Plan{}, err
	}
	return ingest.RenameFromStore(ctx, w.Sess, st, dir, sourceRef, destRef, w.VM)
}

// RenameFromResult plans a rename/move from a Result snapshot (LSP).
// Adapts the snapshot into the store; Walker.Rename does not go this way.
func (w *Walker) RenameFromResult(ctx context.Context, result *project.Result, dir, sourceRef, destRef string) (project.Plan, error) {
	if w == nil || w.Sess == nil {
		return project.Plan{}, ingest.ErrNilSession
	}
	if w.VM == nil {
		return project.Plan{}, fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	return ingest.RenameFromResult(ctx, w.Sess, result, dir, sourceRef, destRef, w.VM)
}

// RewriteImportsInFile updates import paths in one consumer for oldRef → newRef.
func (w *Walker) RewriteImportsInFile(fileRelPath string, content []byte, result *project.Result, oldRef, newRef string) []project.Edit {
	if w == nil || w.VM == nil {
		return nil
	}
	return ingest.RewriteImportsInFile(w.VM, fileRelPath, content, result, oldRef, newRef)
}

// ValidateStaged loads the project through fsys (overlay) and checks atom spans.
func (w *Walker) ValidateStaged(ctx context.Context, dir string, fsys projectfs.FS) error {
	if w == nil || w.Sess == nil {
		return ingest.ErrNilSession
	}
	if w.VM == nil {
		return fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	result, err := w.Load(ctx, ingest.SourceProject(dir).WithFS(fsys), ingest.MaterializeOptions{
		ExpandImports: true,
		FS:            fsys,
	})
	if err != nil {
		return err
	}
	return ingest.ValidateStagedResult(ctx, dir, fsys, result)
}
