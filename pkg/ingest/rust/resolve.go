package rust

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingest"
)

// resolveRustImportSpec maps a Rust use/mod path to a product reference.
//
// Supported:
//   - mod:name          → name.rs / name/mod.rs near importer or crate root
//   - crate::a::b       → module path under crate source root
//   - self::a / super::a → relative to importer module
//   - bare a::b         → project module if known, else rust:a::b
//   - std / external    → rust:…
func resolveRustImportSpec(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}

	if strings.HasPrefix(spec, "mod:") {
		name := strings.TrimPrefix(spec, "mod:")
		if ref, ok := resolveModuleName(name, ctx); ok {
			return ref, true
		}
		return "", false
	}

	// External-looking paths (std, core, alloc, third_party crates).
	if isExternalRustPath(spec, ctx) {
		return "rust:" + spec, true
	}

	segs := splitRustPath(spec)
	if len(segs) == 0 {
		return "", false
	}

	switch segs[0] {
	case "crate":
		root := crateSourceRoot(ctx)
		return resolveModuleSegs(root, segs[1:], ctx)
	case "self":
		base := moduleDirOf(ctx.ImporterPath)
		return resolveModuleSegs(base, segs[1:], ctx)
	case "super":
		base := moduleDirOf(ctx.ImporterPath)
		base = parentDir(base)
		return resolveModuleSegs(base, segs[1:], ctx)
	default:
		// Bare path: try as module from crate root, then importer dir.
		if ref, ok := resolveModuleSegs(crateSourceRoot(ctx), segs, ctx); ok {
			return ref, true
		}
		if ref, ok := resolveModuleSegs(moduleDirOf(ctx.ImporterPath), segs, ctx); ok {
			return ref, true
		}
		if ref, ok := resolveModuleName(segs[0], ctx); ok && len(segs) == 1 {
			return ref, true
		}
		return "rust:" + spec, true
	}
}

func isExternalRustPath(spec string, ctx ingest.ImportResolveContext) bool {
	segs := splitRustPath(spec)
	if len(segs) == 0 {
		return false
	}
	first := segs[0]
	switch first {
	case "crate", "self", "super", "mod":
		return false
	case "std", "core", "alloc", "proc_macro", "test":
		return true
	}
	// If first segment resolves to a project module, not external.
	if _, ok := resolveModuleName(first, ctx); ok {
		return false
	}
	if _, ok := resolveModuleSegs(crateSourceRoot(ctx), []string{first}, ctx); ok {
		return false
	}
	// No project file → treat as external crate path.
	return true
}

func resolveModuleName(name string, ctx ingest.ImportResolveContext) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	// Prefer importer directory (sibling modules), then crate root.
	for _, base := range []string{moduleDirOf(ctx.ImporterPath), crateSourceRoot(ctx), ""} {
		if ref, ok := moduleFileRef(base, name, ctx); ok {
			return ref, true
		}
	}
	return "", false
}

func resolveModuleSegs(base string, segs []string, ctx ingest.ImportResolveContext) (string, bool) {
	if len(segs) == 0 {
		// crate / self alone → crate root file (lib.rs/main.rs) or directory ref
		if ref, ok := crateRootFileRef(base, ctx.KnownFiles); ok {
			return ref, true
		}
		if base == "" || base == "." {
			return ingest.FileRef("./"), true
		}
		return ingest.FileRef("./" + base), true
	}
	// Walk intermediate segments as directories; last as file or dir/mod.rs.
	cur := base
	for i, seg := range segs {
		last := i == len(segs)-1
		if last {
			if ref, ok := moduleFileRef(cur, seg, ctx); ok {
				return ref, true
			}
			// Fall back to path even if not yet on disk (new module moves).
			rel := joinRel(cur, seg+".rs")
			return ingest.FileRef("./" + rel), true
		}
		// Intermediate: enter directory segment
		cur = joinRel(cur, seg)
	}
	return "", false
}

func moduleFileRef(base, name string, ctx ingest.ImportResolveContext) (string, bool) {
	file := strings.TrimPrefix(joinRel(base, name+".rs"), "./")
	if ctx.KnownFiles != nil && ctx.KnownFiles[file] {
		return ingest.FileRef("./" + file), true
	}
	dir := joinRel(base, name)
	if child, ok := ingest.KnownDirRepresentant(ctx.Representant, dir, ctx.KnownFiles); ok {
		return ingest.FileRef("./" + child), true
	}
	return "", false
}

func crateRootFileRef(base string, known map[string]bool) (string, bool) {
	for _, name := range []string{"lib.rs", "main.rs"} {
		rel := joinRel(base, name)
		rel = strings.TrimPrefix(rel, "./")
		if known != nil && known[rel] {
			return ingest.FileRef("./" + rel), true
		}
	}
	return "", false
}

func crateSourceRoot(ctx ingest.ImportResolveContext) string {
	imp := strings.TrimPrefix(filepath.ToSlash(ctx.ImporterPath), "./")
	// Prefer directory containing lib.rs or main.rs among known files.
	if ctx.KnownFiles != nil {
		// If any src/lib.rs or src/main.rs, source root is src.
		if ctx.KnownFiles["src/lib.rs"] || ctx.KnownFiles["src/main.rs"] {
			// Importer under src/?
			if imp == "src/lib.rs" || imp == "src/main.rs" || strings.HasPrefix(imp, "src/") {
				return "src"
			}
			return "src"
		}
		// Flat layout: lib.rs/main.rs at root
		if ctx.KnownFiles["lib.rs"] || ctx.KnownFiles["main.rs"] {
			return ""
		}
		// Walk up from importer looking for sibling lib.rs/main.rs
		dir := path.Dir(imp)
		for {
			for _, name := range []string{"lib.rs", "main.rs"} {
				cand := joinRel(dir, name)
				cand = strings.TrimPrefix(cand, "./")
				if ctx.KnownFiles[cand] {
					if dir == "." {
						return ""
					}
					return dir
				}
			}
			if dir == "." || dir == "" {
				break
			}
			parent := path.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	// Disk: look for Cargo.toml and src/
	if ctx.RootDir != "" {
		if root := findCrateRootOnDisk(ctx.RootDir, imp); root != "" {
			return root
		}
	}
	return moduleDirOf(imp)
}

func findCrateRootOnDisk(rootDir, importerRel string) string {
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return ""
	}
	start := lewpath.New(rootAbs, filepath.FromSlash(strings.TrimPrefix(importerRel, "./"))).String()
	dir := start
	if st, err := os.Stat(dir); err == nil && !st.IsDir() {
		dir = filepath.Dir(dir)
	}
	for {
		// source root: this dir has lib.rs/main.rs
		for _, name := range []string{"lib.rs", "main.rs"} {
			if st, err := os.Stat(lewpath.New(dir, name).String()); err == nil && !st.IsDir() {
				rel, err := filepath.Rel(rootAbs, dir)
				if err != nil {
					return ""
				}
				rel = filepath.ToSlash(rel)
				if rel == "." {
					return ""
				}
				return rel
			}
		}
		// cargo package root with src/
		if st, err := os.Stat(lewpath.New(dir, "Cargo.toml").String()); err == nil && !st.IsDir() {
			src := lewpath.New(dir, "src").String()
			if st, err := os.Stat(src); err == nil && st.IsDir() {
				rel, err := filepath.Rel(rootAbs, src)
				if err != nil {
					return ""
				}
				return filepath.ToSlash(rel)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir || !strings.HasPrefix(parent, rootAbs) {
			break
		}
		dir = parent
	}
	return ""
}

func moduleDirOf(importerPath string) string {
	p := strings.TrimPrefix(filepath.ToSlash(importerPath), "./")
	dir := path.Dir(p)
	if dir == "." {
		return ""
	}
	return dir
}

func parentDir(dir string) string {
	if dir == "" || dir == "." {
		return ""
	}
	p := path.Dir(dir)
	if p == "." {
		return ""
	}
	return p
}

func joinRel(base, elem string) string {
	elem = strings.TrimPrefix(elem, "./")
	if base == "" || base == "." {
		return elem
	}
	return path.Join(base, elem)
}
