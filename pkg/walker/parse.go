package walker

import (
	"context"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
)

// ParseAttributed attributes relPath via the VM, then parses with that host.
func (w *Walker) ParseAttributed(ctx context.Context, content []byte, relPath string) (*ingestutil.ParsedFile, string, error) {
	if w == nil {
		return nil, "", ingest.ErrNilSession
	}
	return pattern.ParseAttributed(ctx, w.Sess, w.VM, content, relPath)
}
