package walker

import (
	"context"
	"fmt"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter"
)

// AbstractFile builds abstract units for a parsed file (no product-ref targets).
// Policy comes from this Walker. Prefer AbstractFileResult when a *Result is
// available so @refs resolve.
func (w *Walker) AbstractFile(ctx context.Context, root *sitter.Node, source []byte, filePath string, onlyFuncs bool) ([]ingest.AbstractUnit, error) {
	return w.AbstractFileResult(ctx, root, source, filePath, onlyFuncs, nil)
}

// AbstractFileResult is AbstractFile with optional ingest Result for Use targets.
func (w *Walker) AbstractFileResult(ctx context.Context, root *sitter.Node, source []byte, filePath string, onlyFuncs bool, result *project.Result) ([]ingest.AbstractUnit, error) {
	if w == nil || w.Sess == nil {
		return nil, ingest.ErrNilSession
	}
	if w.VM == nil {
		return nil, fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	return ingest.AbstractFileResult(ctx, w.VM, w.Sess, root, source, filePath, onlyFuncs, result), nil
}
