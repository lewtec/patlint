package zig

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
)

// resolveZigImportSpec maps an @import("…") string to a product reference.
//
//   - "./foo.zig", "../bar.zig" → path: relative to importer
//   - "foo.zig" with known file near importer/root → path:
//   - bare "std", "builtin", "root", unknown → zig:<spec>
func resolveZigImportSpec(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}

	// Relative path import.
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return resolveRelativeZig(spec, ctx), true
	}
	if strings.HasPrefix(spec, "/") {
		// Absolute — unusual; keep as path if known, else zig:
		rel := strings.TrimPrefix(filepath.ToSlash(spec), "/")
		if ctx.KnownFiles[rel] {
			return "path:./" + rel, true
		}
		return "zig:" + spec, true
	}

	// Explicit .zig file name
	if strings.HasSuffix(spec, ".zig") {
		if ref, ok := findKnownZig(spec, ctx); ok {
			return ref, true
		}
		// try relative to importer as ./spec
		return resolveRelativeZig("./"+spec, ctx), true
	}

	// Build-system package name or std — not a project path.
	switch spec {
	case "std", "builtin", "root", "c":
		return "zig:" + spec, true
	}

	// Bare package/module name: project file <name>.zig if present.
	if ref, ok := findKnownZig(spec+".zig", ctx); ok {
		return ref, true
	}
	return "zig:" + spec, true
}

func resolveRelativeZig(spec string, ctx ingest.ImportResolveContext) string {
	base := path.Dir(filepath.ToSlash(ctx.ImporterPath))
	if base == "." {
		base = ""
	}
	joined := path.Clean(path.Join(base, spec))
	joined = strings.TrimPrefix(joined, "./")
	if joined == "" || joined == "." {
		return "path:./"
	}
	// Prefer known files map for exact match (forward slashes).
	if ctx.KnownFiles != nil {
		if ctx.KnownFiles[joined] {
			return "path:./" + joined
		}
		if ctx.KnownFiles["./"+joined] {
			return "path:./" + joined
		}
	}
	return "path:./" + joined
}

func findKnownZig(fileName string, ctx ingest.ImportResolveContext) (string, bool) {
	fileName = strings.TrimPrefix(filepath.ToSlash(fileName), "./")
	if fileName == "" {
		return "", false
	}
	// Same directory as importer first.
	dir := path.Dir(filepath.ToSlash(ctx.ImporterPath))
	if dir == "." {
		dir = ""
	}
	candidates := []string{
		path.Join(dir, fileName),
		fileName,
		path.Join("src", fileName),
	}
	for _, c := range candidates {
		c = strings.TrimPrefix(path.Clean(c), "./")
		if ctx.KnownFiles[c] || ctx.KnownFiles["./"+c] {
			return "path:./" + c, true
		}
	}
	return "", false
}
