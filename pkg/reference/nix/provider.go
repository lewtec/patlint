package nixref

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
)

type Target struct {
	Dir   string
	File  string
	IsDir bool
}

type SymbolTarget struct {
	Dir  string
	Name string
}

// ResolveTarget resolves a nix:<spec> path through NIX_PATH.
func ResolveTarget(spec string) (Target, error) {
	spec = normalizeSpec(spec)
	if spec == "" {
		return Target{}, ErrEmptyPath
	}

	resolved, err := resolveFromNixPath(spec)
	if err != nil {
		return Target{}, err
	}

	st, err := os.Stat(resolved)
	if err != nil {
		return Target{}, fmt.Errorf("nix path target invalid: %w", err)
	}

	if st.IsDir() {
		target := Target{Dir: resolved, IsDir: true}
		// `default.nix` is the backing module file for a Nix directory scope,
		// analogous to `__init__.py` or `index.js` in other ecosystems.
		if initFile := lewpath.New(resolved, "default.nix").String(); fileExists(initFile) {
			target.File = "default.nix"
		}
		return target, nil
	}

	return Target{
		Dir:  filepath.Dir(resolved),
		File: filepath.Base(resolved),
	}, nil
}

// ResolveSymbolTarget resolves nix:<spec>::<symbol> to an ingest target.
func ResolveSymbolTarget(_ context.Context, spec, symbol string) (SymbolTarget, bool, error) {
	if strings.TrimSpace(symbol) == "" {
		return SymbolTarget{}, false, nil
	}

	target, err := ResolveTarget(spec)
	if err != nil {
		return SymbolTarget{}, true, err
	}

	return SymbolTarget{
		Dir:  target.Dir,
		Name: symbol,
	}, true, nil
}

// MatchesEntityPath reports whether entPath belongs to the resolved target.
func MatchesEntityPath(target Target, entPath string) bool {
	if target.File == "" {
		return true
	}
	return filepath.ToSlash(entPath) == filepath.ToSlash(target.File)
}

func resolveFromNixPath(spec string) (string, error) {
	rootName, subpath := splitProviderSpec(spec)
	if rootName == "" {
		return "", ErrEmptyPath
	}

	for _, entry := range filepath.SplitList(os.Getenv("NIX_PATH")) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		if name, value, ok := strings.Cut(entry, "="); ok {
			if name != rootName {
				continue
			}
			candidate := value
			if subpath != "" {
				candidate = lewpath.New(candidate, filepath.FromSlash(subpath)).String()
			}
			if resolved, ok := existingPath(candidate); ok {
				return resolved, nil
			}
			continue
		}

		candidate := lewpath.New(entry, filepath.FromSlash(spec)).String()
		if resolved, ok := existingPath(candidate); ok {
			return resolved, nil
		}
	}

	return "", fmt.Errorf("%w: %s", ErrPathNotFound, spec)
}

func splitProviderSpec(spec string) (rootName, subpath string) {
	spec = normalizeSpec(spec)
	if spec == "" {
		return "", ""
	}

	parts := strings.Split(spec, "/")
	rootName = parts[0]
	if len(parts) > 1 {
		subpath = strings.Join(parts[1:], "/")
	}
	return rootName, subpath
}

func normalizeSpec(spec string) string {
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "<") && strings.HasSuffix(spec, ">") {
		spec = strings.TrimPrefix(strings.TrimSuffix(spec, ">"), "<")
	}
	spec = strings.Trim(spec, "/")
	return spec
}

func existingPath(candidate string) (string, bool) {
	if candidate == "" {
		return "", false
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	if _, err := os.Stat(abs); err != nil {
		return "", false
	}
	return abs, true
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
