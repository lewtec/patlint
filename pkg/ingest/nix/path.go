package nix

import (
	"path"
	"path/filepath"
	"strings"
)

func relativeNixPath(importerDir, newPath, originalSpec string) string {
	newPath = strings.TrimPrefix(filepath.ToSlash(newPath), "./")
	target := newPath
	orig := strings.TrimSpace(originalSpec)
	// Original was a directory-style import: rewrite to dir (or ./. for same dir root).
	dirStyle := orig == "." || orig == "./." || !strings.HasSuffix(orig, ".nix")
	if dirStyle && path.Base(newPath) == "default.nix" {
		target = path.Dir(newPath)
		if target == "." {
			target = ""
		}
	}

	var rel string
	if importerDir == "" || importerDir == "." {
		if target == "" {
			return "./."
		}
		rel = target
	} else {
		r, err := filepath.Rel(filepath.FromSlash(importerDir), filepath.FromSlash(func() string {
			if target == "" {
				return "."
			}
			return target
		}()))
		if err != nil {
			if target == "" {
				return "./."
			}
			return "./" + target
		}
		rel = filepath.ToSlash(r)
	}
	if rel == "." {
		return "./."
	}
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel
}
