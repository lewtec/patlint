package html

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
)

const FamilyID = "html"

// Family is the HTML family (no move lattice yet).
var Family = ingest.RegisterFamily(FamilyID, ingest.FamilySpec{
	ResolveImport: resolveHTMLImport,
})

func resolveHTMLImport(sourcePath string, _ ingest.ImportResolveContext) string {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return ""
	}
	return "path:" + sourcePath
}
