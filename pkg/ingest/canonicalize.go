package ingest

import (
	"github.com/lewtec/patlint/pkg/projectfs"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
)

// CanonicalizeReference turns a reference into the preferred form for navigation
// and inspection. It is provider-agnostic at the logic layer: after a small
// filesystem prelude for path dirs, it ingests a Result and walks only
// Atoms / Aliases (the ingestor graph), which any provider/language shares.
//
// Prelude (path only, to obtain a concrete seed file):
//   - normalize ./ prefix
//   - directory → module entry via pack as-directory-representant / family directory-manifest
//
// Graph walk (any provider, via Result — see CanonicalizeInResult):
//   - exact Atom hit
//   - module/file ref with no symbol → sole entity in that path (default/primary export)
//   - symbol not defined at this path → Alias forwards from this scope (re-exports/barrels)
//   - else sole entity with that symbol anywhere in the Result
//
// When the input uses a non-path provider (go:, python:, …), the final reference
// keeps that provider/path and only updates Symbol (entities in Result are path-shaped).
//
// Missing ingest / unresolvable hops return the best ref found so far (no error).
// Hop loads are Walker.CanonicalizeReference (VM PackQueries).

// CanonicalizeInResult walks only the provider-agnostic ingest graph (Atoms,
// Aliases). No filesystem or language driver calls — callers supply an already
// built Result (e.g. from MaterializeSource / Seed / Dir drivers).
//
// Intermediate alias/entity targets may be path: refs even when the input was
// go:/python:; use Walker.CanonicalizeReference (or ProjectToInputProvider) to restore
// the caller's provider wrapper.
func CanonicalizeInResult(result *project.Result, ref Reference) Reference {
	if result == nil {
		return ref
	}
	ref = NormalizePathReference(ref)
	if ref.Provider == "" {
		ref.Provider = "path"
	}

	const maxHops = 16
	seen := map[string]bool{}
	for hop := 0; hop < maxHops; hop++ {
		key := ref.String()
		if seen[key] {
			return ref
		}
		seen[key] = true

		if ent, ok := entityExact(result, key); ok {
			return ParseReference(ent.Reference)
		}

		if ref.Name == "" {
			// Module/file ref: prefer an alias hop to a same-scope symbol (drivers
			// record primary/default export this way). Sole entity is fallback only.
			if next, ok := followSameScopeSymbolAlias(result, ref); ok {
				ref = next
				continue
			}
			if next, ok := soleEntityInScope(result, ref); ok {
				ref = next
				continue
			}
			return ref
		}

		if ent, ok := entityAtPathSymbol(result, ref); ok {
			return ParseReference(ent.Reference)
		}

		if next, ok := followAliasForward(result, ref); ok {
			ref = next
			continue
		}

		// ESM "default" often means the module's primary export (DefaultExport alias
		// or sole entity), not a symbol literally named "default".
		if ref.Name == "default" {
			bare := ref
			bare.Name = ""
			if next, ok := followSameScopeSymbolAlias(result, bare); ok {
				ref = next
				continue
			}
			if next, ok := soleEntityInScope(result, bare); ok {
				ref = next
				continue
			}
		}

		if ent, ok := SoleEntityNamed(result, ref.Name); ok {
			return ParseReference(ent.Reference)
		}

		return ref
	}
	return ref
}

// ProjectToInputProvider keeps non-path providers (go:fmt) instead of leaking
// path:./print.go entities from the ingest Result.
func ProjectToInputProvider(origProvider, origPath string, out Reference) Reference {
	if origProvider == "" || origProvider == "path" {
		return out
	}
	return Reference{
		Provider: origProvider,
		Path:     origPath,
		Name:     out.Name,
	}
}

// CanonicalizePathReference is the directory→module-entry step only (path provider).
// Prefer Walker.CanonicalizeReference for full graph canonicalization.
func CanonicalizePathReference(policy PackQueries, baseDir string, ref Reference) Reference {
	ref = NormalizePathReference(ref)
	if ref.Provider != "path" {
		return ref
	}
	if baseDir == "" {
		baseDir = "."
	}
	rootAbs, err := filepath.Abs(baseDir)
	if err != nil {
		rootAbs = baseDir
	}
	return canonicalizeDirectoryModule(policy, rootAbs, ref)
}

func canonicalizeDirectoryModule(policy PackQueries, rootAbs string, ref Reference) Reference {
	if ref.Provider != "path" || ref.Path == "" || ref.Path == "./" {
		return ref
	}
	rel := strings.TrimPrefix(ref.Path, "./")
	abs := ref.Path
	if !filepath.IsAbs(abs) {
		abs = lewpath.New(rootAbs, filepath.FromSlash(rel)).String()
	}
	st, err := (projectfs.OS{}).Stat(abs)
	if err != nil || !st.IsDir() {
		return ref
	}
	entry, ok := ResolveDirectoryModuleFile(policy, abs, rel)
	if !ok {
		return ref
	}
	entry = filepath.ToSlash(entry)
	ref.Path = "./" + pathJoinSlash(rel, entry)
	return ref
}

func entityExact(result *project.Result, refStr string) (project.Atom, bool) {
	for _, ent := range result.Atoms {
		if ent.Reference == refStr {
			return ent, true
		}
	}
	return project.Atom{}, false
}

func entityAtPathSymbol(result *project.Result, ref Reference) (project.Atom, bool) {
	for _, ent := range result.Atoms {
		er := ParseReference(ent.Reference)
		if er.Name != ref.Name {
			continue
		}
		if SameScopePath(ref, er) {
			return ent, true
		}
	}
	return project.Atom{}, false
}

func soleEntityInScope(result *project.Result, ref Reference) (Reference, bool) {
	var matches []project.Atom
	for _, ent := range result.Atoms {
		er := ParseReference(ent.Reference)
		if SameScopePath(ref, er) {
			matches = append(matches, ent)
		}
	}
	if len(matches) == 1 {
		return ParseReference(matches[0].Reference), true
	}
	return Reference{}, false
}

func SoleEntityNamed(result *project.Result, symbol string) (project.Atom, bool) {
	var matches []project.Atom
	for _, ent := range result.Atoms {
		er := ParseReference(ent.Reference)
		if er.Name == symbol {
			matches = append(matches, ent)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return project.Atom{}, false
}

// followSameScopeSymbolAlias finds an alias on this scope whose target is a
// symbol still in the same path/provider (module primary export), not a re-export
// to another module.
func followSameScopeSymbolAlias(result *project.Result, ref Reference) (Reference, bool) {
	for _, a := range result.Aliases {
		ar := ParseReference(a.Reference)
		if !SameScopePath(ref, ar) {
			continue
		}
		tr := ParseReference(a.Target)
		if tr.Name == "" {
			continue
		}
		if SameScopePath(ref, tr) {
			return tr, true
		}
	}
	return Reference{}, false
}

// followAliasForward uses Alias targets as provider-agnostic hops (imports and
// re-exports are both recorded as aliases in the Result).
func followAliasForward(result *project.Result, ref Reference) (Reference, bool) {
	var starHop Reference
	var hasStar bool

	for _, a := range result.Aliases {
		ar := ParseReference(a.Reference)
		if !SameScopePath(ref, ar) {
			continue
		}
		// Named reexport/import binding: alias reference carries the local export name.
		if ar.Name != "" {
			if ref.Name == "" || ar.Name != ref.Name {
				continue
			}
			return ParseReference(a.Target), true
		}
		tr := ParseReference(a.Target)
		// Legacy/module-level alias whose target already names the requested symbol
		// (e.g. file → other::Search).
		if ref.Name != "" && tr.Name == ref.Name {
			return tr, true
		}
		// Star re-export only: zero-span module→module forward (export * from).
		// Import bindings also use file-scoped aliases with spans — do not treat
		// those as star hops for arbitrary symbols (would send default→wrong module).
		if ref.Name != "" && ar.Name == "" && tr.Name == "" && a.StartByte == 0 && a.EndByte == 0 {
			tr.Name = ref.Name
			if !hasStar {
				starHop = tr
				hasStar = true
			}
		}
	}
	if hasStar {
		return starHop, true
	}
	return Reference{}, false
}

// SameScopePath compares provider+path identity for scope (ignores symbol).
// Non-path providers (go:fmt) do not match path:./print.go entity scopes; alias
// hops use path refs in the Result, so followAliasForward only applies when ref
// itself is path-scoped during intermediate hops.
func SameScopePath(a, b Reference) bool {
	ap := a.Provider
	if ap == "" {
		ap = "path"
	}
	bp := b.Provider
	if bp == "" {
		bp = "path"
	}
	if ap != bp {
		return false
	}
	return normalizeRelPath(a.Path) == normalizeRelPath(b.Path)
}

func normalizeRelPath(p string) string {
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimSuffix(p, "/")
	if p == "." {
		return ""
	}
	return p
}
