package ingest

import (
	"context"

	"github.com/lewtec/patlint/pkg/project"
)

// resolve is Materialize stratum 2. Store + Datalog; Result is a projection.
func resolve(ctx context.Context, rootDir string, extracts []*project.FileExtract, policy PackQueries) (*project.Result, error) {
	return evalGraph(ctx, rootDir, extracts, policy)
}
