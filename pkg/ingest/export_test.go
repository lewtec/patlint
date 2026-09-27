package ingest

import (
	"github.com/lewtec/patlint/pkg/project"
	// Test helpers — only visible to tests in package ingest_test.
)

func TargetMatchesPackageSymbolForTest(rootDir string, ref Reference, pkgDir string) bool {
	return targetMatchesPackageSymbol(rootDir, ref, pkgDir)
}

func ExpandRenameSourceSetForTest(rootDir string, result *project.Result, sourceRefs []string) StringSet {
	return expandRenameSourceSet(rootDir, result, sourceRefs)
}
