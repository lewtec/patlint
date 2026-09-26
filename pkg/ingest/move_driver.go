package ingest

import (
	"path"
	"strings"

	"github.com/lewtec/patlint/pkg/project"
)

// DeclExtract is one declaration hole + text for a cross-file move.
type DeclExtract struct {
	Preamble    string
	DeclText    string
	Imports     []string
	RemoveStart uint32
	RemoveEnd   uint32
}

// RewriteImportsInFile rewrites marked as-import path tokens for a file/dir move.
func RewriteImportsInFile(policy PackQueries, fileRelPath string, content []byte, result *project.Result, oldRef, newRef string) []project.Edit {
	old := ParseReference(oldRef)
	neu := ParseReference(newRef)
	if result != nil {
		oldKey, newKey := importRewriteKeys(old, neu)
		if oldKey != "" && newKey != "" && oldKey != newKey {
			if edits := RewriteMarkedImportDir(fileRelPath, content, result, oldKey, newKey); len(edits) > 0 {
				return edits
			}
		}
	}
	return nil
}

func importRewriteKeys(old, neu Reference) (oldKey, newKey string) {
	oldKey = strings.TrimPrefix(filepathToSlashImport(old.Path), "./")
	newKey = strings.TrimPrefix(filepathToSlashImport(neu.Path), "./")
	if old.Name != "" {
		oldKey = path.Dir(oldKey)
		newKey = path.Dir(newKey)
	}
	if oldKey == "." {
		oldKey = ""
	}
	if newKey == "." {
		newKey = ""
	}
	return oldKey, newKey
}
