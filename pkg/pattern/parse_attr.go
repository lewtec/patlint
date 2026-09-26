package pattern

import (
	"fmt"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"path/filepath"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
)

// ParseAttributed attributes relPath via VM PackQueries, then parses with that host.
func ParseAttributed(sess *project.Session, vm *LispVM, content []byte, relPath string) (*ingestutil.ParsedFile, string, error) {
	if sess == nil {
		return nil, "", ingest.ErrNilSession
	}
	if vm == nil {
		return nil, "", fmt.Errorf("%w: walker: nil LispVM", ErrExtract)
	}
	rel := filepath.ToSlash(relPath)
	lang, ok, err := vm.AttributeHost(rel)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", fmt.Errorf("%w for %s (no pack path claims this file)", ingest.ErrUnsupportedLanguage, relPath)
	}
	pf, err := ingestutil.ParseSource(sess.Engine(), content, relPath, vm.GrammarForLanguage(lang))
	if err != nil {
		return nil, lang, err
	}
	return pf, lang, nil
}
