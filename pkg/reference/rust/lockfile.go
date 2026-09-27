package rustref

import (
	"fmt"
	"path/filepath"

	"github.com/lewtec/patlint/pkg/projectfs"
	"strings"
	"sync"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/pelletier/go-toml/v2"
)

// LockPackage is one [[package]] entry in Cargo.lock.
type LockPackage struct {
	Name         string   `toml:"name"`
	Version      string   `toml:"version"`
	Source       string   `toml:"source,omitempty"`
	Checksum     string   `toml:"checksum,omitempty"`
	Dependencies []string `toml:"dependencies,omitempty"`
}

// Lockfile is a parsed Cargo.lock (v1–v4 TOML schema).
//
// It is a small utility type for "which version of crate X is locked", not a
// full Cargo client. Prefer this over scanning ~/.cargo/registry for a
// lexicographically latest folder name.
type Lockfile struct {
	FormatVersion int           `toml:"version"`
	Packages      []LockPackage `toml:"package"`

	// byName indexes package name → entries (hyphen/underscore folded).
	byName map[string][]LockPackage
}

// LoadLockfile reads and parses path (must be a Cargo.lock file).
func LoadLockfile(path string) (*Lockfile, error) {
	data, err := (projectfs.OS{}).ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseLockfile(data)
}

// ParseLockfile parses Cargo.lock TOML bytes.
func ParseLockfile(data []byte) (*Lockfile, error) {
	var lf Lockfile
	if err := toml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse Cargo.lock: %w", err)
	}
	lf.index()
	return &lf, nil
}

// FindLockfile walks up from startDir looking for Cargo.lock.
// Returns the absolute path and parsed lockfile, or nil if none.
func FindLockfile(startDir string) (path string, lf *Lockfile, err error) {
	if startDir == "" {
		return "", nil, nil
	}
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", nil, err
	}
	for {
		cand := lewpath.New(dir, "Cargo.lock").String()
		if st, err := (projectfs.OS{}).Stat(cand); err == nil && !st.IsDir() {
			lf, err := LoadLockfile(cand)
			if err != nil {
				return cand, nil, err
			}
			return cand, lf, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil, nil
		}
		dir = parent
	}
}

func (lf *Lockfile) index() {
	if lf == nil {
		return
	}
	lf.byName = make(map[string][]LockPackage, len(lf.Packages))
	for _, p := range lf.Packages {
		if p.Name == "" {
			continue
		}
		key := normalizeCrateName(p.Name)
		lf.byName[key] = append(lf.byName[key], p)
	}
}

// PackagesNamed returns all locked packages with the given crate name
// (hyphen/underscore insensitive).
func (lf *Lockfile) PackagesNamed(name string) []LockPackage {
	if lf == nil || lf.byName == nil {
		return nil
	}
	return lf.byName[normalizeCrateName(name)]
}

// Version returns a single locked version for name when unambiguous.
//
// Rules:
//  1. Exactly one package with that name → its version.
//  2. Several versions → prefer packages reachable from workspace/path
//     members (entries with empty source), following dependency edges.
//  3. Still ambiguous → ok=false (caller must not guess).
func (lf *Lockfile) Version(name string) (version string, ok bool) {
	pkgs := lf.PackagesNamed(name)
	switch len(pkgs) {
	case 0:
		return "", false
	case 1:
		return pkgs[0].Version, pkgs[0].Version != ""
	}

	// Multiple versions: resolve via graph from path/workspace roots.
	used := lf.versionsReachableFromRoots()
	key := normalizeCrateName(name)
	vers := used[key]
	switch len(vers) {
	case 0:
		// Name not reachable from roots (orphan / build-dep only): if all
		// entries share one version string, still ok; else refuse.
		return uniqueVersion(pkgs)
	case 1:
		for v := range vers {
			return v, true
		}
	}
	return "", false
}

// versionsReachableFromRoots maps normalizeCrateName → set of versions
// reachable from packages that have no source (workspace / path members).
func (lf *Lockfile) versionsReachableFromRoots() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	if lf == nil {
		return out
	}

	// id key: name@version
	type id struct{ name, ver string }
	byID := map[id]LockPackage{}
	// bare name → packages when unique for dep matching
	for _, p := range lf.Packages {
		byID[id{normalizeCrateName(p.Name), p.Version}] = p
	}

	var roots []LockPackage
	for _, p := range lf.Packages {
		if p.Source == "" && p.Name != "" {
			roots = append(roots, p)
		}
	}
	if len(roots) == 0 {
		// Library lock without path members: treat every package as a root seed
		// only for unique names; multi-version stays unresolved at Version().
		return out
	}

	seen := map[id]bool{}
	var queue []LockPackage
	for _, r := range roots {
		queue = append(queue, r)
		seen[id{normalizeCrateName(r.Name), r.Version}] = true
	}

	add := func(name, ver string) {
		n := normalizeCrateName(name)
		if out[n] == nil {
			out[n] = map[string]bool{}
		}
		out[n][ver] = true
	}

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		add(p.Name, p.Version)
		for _, dep := range p.Dependencies {
			dn, dv, ok := parseDepSpec(dep)
			if !ok {
				continue
			}
			dn = normalizeCrateName(dn)
			var candidates []LockPackage
			if dv != "" {
				if c, ok := byID[id{dn, dv}]; ok {
					candidates = []LockPackage{c}
				}
			} else {
				candidates = lf.byName[dn]
			}
			for _, c := range candidates {
				cid := id{normalizeCrateName(c.Name), c.Version}
				if seen[cid] {
					continue
				}
				// Bare dep name with multiple versions: skip ambiguous edge
				// (Cargo would have written a qualified dep string).
				if dv == "" && len(candidates) > 1 {
					continue
				}
				seen[cid] = true
				queue = append(queue, c)
			}
		}
	}
	return out
}

func uniqueVersion(pkgs []LockPackage) (string, bool) {
	if len(pkgs) == 0 {
		return "", false
	}
	v := pkgs[0].Version
	for _, p := range pkgs[1:] {
		if p.Version != v {
			return "", false
		}
	}
	return v, v != ""
}

// parseDepSpec parses a Cargo.lock dependency string.
//
//	"memchr"
//	"memchr 2.7.4"
//	"memchr 2.7.4 (registry+https://github.com/rust-lang/crates.io-index)"
func parseDepSpec(dep string) (name, version string, ok bool) {
	dep = strings.TrimSpace(dep)
	if dep == "" {
		return "", "", false
	}
	// Drop trailing source parenthetical.
	if i := strings.Index(dep, " ("); i >= 0 {
		dep = strings.TrimSpace(dep[:i])
	}
	parts := strings.Fields(dep)
	if len(parts) == 0 {
		return "", "", false
	}
	name = parts[0]
	if len(parts) >= 2 {
		version = parts[1]
	}
	return name, version, true
}

// normalizeCrateName folds cargo package name punctuation for lookup.
// Cargo treats '-' and '_' as equivalent for matching in many contexts.
func normalizeCrateName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ToLower(name)
	return strings.ReplaceAll(name, "_", "-")
}

// DirName returns the on-disk registry folder name for a locked package:
// "{name}-{version}" (Cargo registry layout).
func (p LockPackage) DirName() string {
	if p.Name == "" || p.Version == "" {
		return ""
	}
	return p.Name + "-" + p.Version
}

// --- project lockfile cache (per rootDir) ---

var (
	lockCacheMu sync.Mutex
	lockCache   = map[string]*Lockfile{} // abs dir → lock (nil sentinel not stored)
)

// LockfileForProject finds and caches the Cargo.lock governing startDir.
func LockfileForProject(startDir string) (*Lockfile, error) {
	if startDir == "" {
		return nil, nil
	}
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}
	lockCacheMu.Lock()
	defer lockCacheMu.Unlock()
	if lf, ok := lockCache[abs]; ok {
		return lf, nil
	}
	// Walk once; cache under every visited dir so siblings hit.
	dir := abs
	var found *Lockfile
	var foundPath string
	for {
		if lf, ok := lockCache[dir]; ok {
			found = lf
			break
		}
		cand := lewpath.New(dir, "Cargo.lock").String()
		if st, err := (projectfs.OS{}).Stat(cand); err == nil && !st.IsDir() {
			lf, err := LoadLockfile(cand)
			if err != nil {
				return nil, err
			}
			found = lf
			foundPath = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Cache under start abs (and found path).
	lockCache[abs] = found
	if foundPath != "" {
		lockCache[foundPath] = found
	}
	return found, nil
}
