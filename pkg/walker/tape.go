package walker

import (
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/tape"
)

// BuildTape attributes relPath, parses the host language, and builds the tape.
func (w *Walker) BuildTape(source []byte, relPath string) (cells []tape.Cell, lang string, err error) {
	if w == nil {
		return nil, "", ingest.ErrNilSession
	}
	return pattern.BuildTape(w.Sess, w.VM, source, relPath)
}
