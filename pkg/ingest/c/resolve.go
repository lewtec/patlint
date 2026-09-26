package c

import (
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingest"
)

// ResolveInclude maps #include paths and C++ using/namespace-alias specs.
//
//	"foo.h" / ./../ → path:./… relative to importer
//	<stdio.h> → c:stdio.h
//	util.detail.Box (using target) → c:util.detail.Box (provider; no expand)
func ResolveInclude(sourcePath string, ctx ingest.ImportResolveContext) string {
	spec, system := normalizeIncludeSpec(sourcePath)
	if spec == "" {
		return ""
	}
	if system {
		return "c:" + spec
	}

	// C++ using / namespace-alias targets are dotted names, not headers.
	if !looksLikeHeaderPath(spec) && !strings.HasPrefix(spec, "./") &&
		!strings.HasPrefix(spec, "../") && !strings.HasPrefix(spec, "/") &&
		strings.Contains(spec, ".") {
		return "c:" + spec
	}

	rel := joinImporterInclude(ctx.ImporterPath, spec)
	if rel != "" {
		if ref, ok := resolveKnownCPath(rel, ctx.KnownFiles); ok {
			return ref
		}
		if ref, ok := resolveCPathOnDisk(ctx.RootDir, rel); ok {
			return ref
		}
		return ingest.FileRef("./" + rel)
	}

	if ref, ok := resolveKnownCPath(spec, ctx.KnownFiles); ok {
		return ref
	}
	if ref, ok := resolveCPathOnDisk(ctx.RootDir, spec); ok {
		return ref
	}
	return ingest.FileRef("./" + strings.TrimPrefix(spec, "./"))
}

func looksLikeHeaderPath(spec string) bool {
	spec = strings.TrimSpace(spec)
	switch strings.ToLower(filepath.Ext(spec)) {
	case ".h", ".hh", ".hpp", ".hxx", ".c", ".cc", ".cpp", ".cxx", ".inc", ".inl":
		return true
	default:
		return false
	}
}

func normalizeIncludeSpec(raw string) (spec string, system bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		return strings.TrimSpace(s[1 : len(s)-1]), true
	}
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) >= 2 {
		return strings.TrimSpace(s[1 : len(s)-1]), false
	}
	if strings.HasPrefix(s, "sys:") {
		return strings.TrimPrefix(s, "sys:"), true
	}
	return s, false
}

func joinImporterInclude(importerPath, spec string) string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ""
	}
	if strings.HasPrefix(spec, "/") {
		return strings.TrimPrefix(filepath.ToSlash(spec), "/")
	}
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return strings.TrimPrefix(ingest.RelativeImportPath(importerPath, spec), "./")
	}
	return strings.TrimPrefix(ingest.RelativeImportPath(importerPath, "./"+spec), "./")
}

func resolveKnownCPath(rel string, known map[string]bool) (string, bool) {
	if known == nil {
		return "", false
	}
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	if rel == "" || rel == "." {
		return "", false
	}
	if known[rel] {
		return ingest.FileRef("./" + rel), true
	}
	return "", false
}

func resolveCPathOnDisk(rootDir, rel string) (string, bool) {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	if rel == "" {
		return "", false
	}
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return "", false
	}
	candidate := lewpath.New(rootAbs, filepath.FromSlash(rel)).String()
	st, err := os.Stat(candidate)
	if err != nil || st.IsDir() {
		return "", false
	}
	return ingest.PathReferenceForAbsolute(rootAbs, candidate), true
}
