package ingest

import (
	"strings"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/store"
)

// Package relation projection. Mechanism. No language names, no pack knobs.
//
// Truth is store.RelationPackage. Result.Files.Package is the same row
// after Project. destinationPackageName is the leftover-use qualifier.

func packageNameFromRelationPackage(storeRows *store.Store, fileRelative string) (name string, end uint32, ok bool) {
	if storeRows == nil {
		return "", 0, false
	}
	fileRelative = strings.TrimPrefix(fileRelative, "./")
	for _, row := range storeRows.Rows(store.RelationPackage) {
		if len(row) < 3 || !sameStoreFile(row[0], fileRelative) {
			continue
		}
		return row[1], store.Atoi(row[2]), true
	}
	return "", 0, false
}

func packageName(result *project.Result, fileRel string) string {
	fileRel = strings.TrimPrefix(fileRel, "./")
	if result == nil || fileRel == "" {
		return ""
	}
	for _, f := range result.Files {
		if strings.TrimPrefix(f.Path, "./") == fileRel && f.Package != "" {
			return f.Package
		}
	}
	return ""
}

func destinationPackageName(result *project.Result, destinationRelative, destinationDirectory string) string {
	if name := packageName(result, destinationRelative); name != "" {
		return name
	}
	if destinationDirectory != "" {
		return LastPathComponent(destinationDirectory)
	}
	return fileStem(destinationRelative)
}
