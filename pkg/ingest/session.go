package ingest

import (
	"context"
	"os"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/projectfs"
	"github.com/lewtec/patlint/pkg/store"
)

// parseFileAt parses one file. fsys nil means sess's FS (or OS).
func parseFileAt(ctx context.Context, s *project.Session, rootAbs, absPath string, _ os.FileInfo, fsys projectfs.FS, policy PackQueries, into *store.Store) (*project.FileExtract, error) {
	if fsys == nil {
		fsys = sessionProjectFS(s)
	}
	return parseFile(ctx, s, rootAbs, absPath, fsys, policy, into)
}
