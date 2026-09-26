package zig

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
)

// ResolveImport maps a zig @import spec to a product ref.
func ResolveImport(sourcePath string, ctx ingest.ImportResolveContext) string {
	if ref, ok := resolveZigImportSpec(sourcePath, ctx); ok {
		return ref
	}
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return ""
	}
	return "zig:" + sourcePath
}
