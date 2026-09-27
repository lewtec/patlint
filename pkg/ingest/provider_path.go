package ingest

import (
	"encoding/json"
	"github.com/lewtec/patlint/pkg/projectfs"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	refpkg "github.com/lewtec/patlint/pkg/reference"
)

type pathReferenceProvider struct{}

func NewPathReferenceProvider() refpkg.Provider { return pathReferenceProvider{} }

func init() {
	RegisterReferenceProvider("path", NewPathReferenceProvider())
}

func (pathReferenceProvider) Name() string { return "path" }

func (pathReferenceProvider) Resolve(spec string, ctx ImportResolveContext) (string, bool) {
	if !(strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") || strings.HasPrefix(spec, "/")) {
		return "", false
	}

	rel := RelativeImportPath(ctx.ImporterPath, spec)
	if rel == "" {
		return "", false
	}

	if ref, ok := resolveKnownPathRepresentant(rel, ctx); ok {
		return ref, true
	}

	rootAbs, err := filepath.Abs(ctx.RootDir)
	if err != nil {
		return FileRef("./" + rel), true
	}
	candidate := lewpath.New(rootAbs, filepath.FromSlash(rel)).String()
	resolved, ok := resolvePathFileOnDisk(candidate, rel, ctx)
	if ok {
		return PathReferenceForAbsolute(rootAbs, resolved), true
	}

	return FileRef("./" + rel), true
}

func resolveKnownPathRepresentant(rel string, ctx ImportResolveContext) (string, bool) {
	if ctx.KnownFiles[rel] {
		return FileRef("./" + rel), true
	}
	for _, ext := range []string{".js", ".mjs", ".cjs"} {
		if ctx.KnownFiles[rel+ext] {
			return FileRef("./" + rel + ext), true
		}
	}
	if child, ok := KnownDirRepresentant(ctx.Representant, rel, ctx.KnownFiles); ok {
		return FileRef("./" + child), true
	}
	return "", false
}

func resolvePathFileOnDisk(baseAbs, dirRel string, ctx ImportResolveContext) (string, bool) {
	if st, err := (projectfs.OS{}).Stat(baseAbs); err == nil {
		if !st.IsDir() {
			return baseAbs, true
		}
		if mainEntry, ok := readPackageMain(lewpath.New(baseAbs, "package.json").String()); ok {
			mainAbs := lewpath.New(baseAbs, filepath.FromSlash(mainEntry)).String()
			if resolved, ok := resolvePathFileOnDisk(mainAbs, mainEntry, ctx); ok {
				return resolved, true
			}
		}
		if base, ok := DiskDirRepresentant(ctx.Representant, baseAbs, dirRel); ok {
			return lewpath.New(baseAbs, filepath.FromSlash(base)).String(), true
		}
	}
	for _, ext := range []string{".js", ".mjs", ".cjs"} {
		candidate := baseAbs + ext
		if st, err := (projectfs.OS{}).Stat(candidate); err == nil && !st.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func readPackageMain(packageJSONPath string) (string, bool) {
	data, err := (projectfs.OS{}).ReadFile(packageJSONPath)
	if err != nil {
		return "", false
	}
	var pkg struct {
		Main string `json:"main"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", false
	}
	if pkg.Main == "" {
		return "", false
	}
	return filepath.ToSlash(pkg.Main), true
}
