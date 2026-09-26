package walker

import (
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
)

// ParseAttributed attributes relPath via the VM, then parses with that host.
func (w *Walker) ParseAttributed(content []byte, relPath string) (*ingestutil.ParsedFile, string, error) {
	if w == nil {
		return nil, "", ingest.ErrNilSession
	}
	return pattern.ParseAttributed(w.Sess, w.VM, content, relPath)
}
