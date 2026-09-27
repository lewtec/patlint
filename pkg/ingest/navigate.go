package ingest

import (
	"path/filepath"
	"strconv"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
)

func EntityExactOK(result *project.Result, refStr string) bool {
	_, ok := entityExact(result, refStr)
	return ok
}

func EntityAtPathSymbolOK(result *project.Result, ref Reference) bool {
	_, ok := entityAtPathSymbol(result, ref)
	return ok
}

func AbsPathForRef(root string, ref Reference) string {
	if ref.Provider != "" && ref.Provider != "path" {
		return ""
	}
	if ref.Path == "" {
		return ""
	}
	if filepath.IsAbs(ref.Path) {
		return ref.Path
	}
	rel := strings.TrimPrefix(ref.Path, "./")
	abs, err := filepath.Abs(lewpath.New(root, filepath.FromSlash(rel)).String())
	if err != nil {
		return lewpath.New(root, filepath.FromSlash(rel)).String()
	}
	return abs
}

func FirstAliasTarget(result *project.Result, ref Reference) (Reference, bool) {
	if result == nil {
		return Reference{}, false
	}
	key := ref.String()
	scope := FileRef("./" + strings.TrimPrefix(ref.Path, "./"))
	if ref.Name != "" {
		scope = ref.String()
	}
	for _, a := range result.Aliases {
		if a.Reference == key || a.Reference == scope || (ref.Name == "" && a.Reference == FileRef("./"+strings.TrimPrefix(ref.Path, "./"))) {
			if a.Target != "" {
				return ParseReference(a.Target), true
			}
		}
	}
	// also match path-only aliases when looking for symbol
	for _, a := range result.Aliases {
		ar := ParseReference(a.Reference)
		if SameScopePath(ref, ar) && a.Target != "" {
			t := ParseReference(a.Target)
			if ref.Name != "" && t.Name == "" {
				t.Name = ref.Name
			}
			return t, true
		}
	}
	return Reference{}, false
}

// AbsolutizeResultPaths rewrites path:./rel atoms/aliases/uses/files under rootDir
// to absolute path: refs so navigation outside the project root (module cache,
// GOROOT) still opens the right file.
func AbsolutizeResultPaths(res *project.Result, rootDir string) *project.Result {
	if res == nil || rootDir == "" {
		return res
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = rootDir
	}
	mapRef := func(s string) string {
		if s == "" {
			return s
		}
		r := ParseReference(s)
		if r.Provider != "" && r.Provider != "path" {
			return s
		}
		p := r.Path
		if p == "" || filepath.IsAbs(p) {
			return s
		}
		rel := strings.TrimPrefix(p, "./")
		r.Provider = "path"
		r.Path = filepath.ToSlash(lewpath.New(absRoot, filepath.FromSlash(rel)).String())
		return r.String()
	}
	out := &project.Result{Families: res.Families}
	for _, f := range res.Files {
		p := f.Path
		if p != "" && !filepath.IsAbs(p) {
			rel := strings.TrimPrefix(p, "./")
			f.Path = filepath.ToSlash(lewpath.New(absRoot, filepath.FromSlash(rel)).String())
		}
		out.Files = append(out.Files, f)
	}
	for _, e := range res.Atoms {
		e.Reference = mapRef(e.Reference)
		out.Atoms = append(out.Atoms, e)
	}
	for _, a := range res.Aliases {
		a.Reference = mapRef(a.Reference)
		if a.Target != "" {
			a.Target = mapRef(a.Target)
		}
		out.Aliases = append(out.Aliases, a)
	}
	for _, u := range res.Uses {
		u.Reference = mapRef(u.Reference)
		if u.Target != "" {
			u.Target = mapRef(u.Target)
		}
		out.Uses = append(out.Uses, u)
	}
	return out
}

// MergeResults concatenates files/entities/aliases/relations (dedupe by identity).
func MergeResults(parts ...*project.Result) *project.Result {
	out := &project.Result{}
	seenFile := map[string]bool{}
	seenEnt := map[string]bool{}
	seenAlias := map[string]bool{}
	seenRel := map[string]bool{}
	seenFam := map[string]bool{}
	for _, p := range parts {
		if p == nil {
			continue
		}
		for _, c := range p.Families {
			k := c.Lang + "\x00" + c.Family
			if seenFam[k] {
				continue
			}
			seenFam[k] = true
			out.Families = append(out.Families, c)
		}
		for _, f := range p.Files {
			k := f.Path + "\x00" + f.Language
			if seenFile[k] {
				continue
			}
			seenFile[k] = true
			out.Files = append(out.Files, f)
		}
		for _, e := range p.Atoms {
			k := e.Reference + "\x00" + strconv.FormatUint(uint64(e.StartByte), 10) + "\x00" + strconv.FormatUint(uint64(e.EndByte), 10)
			if seenEnt[k] {
				continue
			}
			seenEnt[k] = true
			out.Atoms = append(out.Atoms, e)
		}
		for _, a := range p.Aliases {
			k := a.Reference + "\x00" + a.Target + "\x00" + strconv.FormatUint(uint64(a.StartByte), 10)
			if seenAlias[k] {
				continue
			}
			seenAlias[k] = true
			out.Aliases = append(out.Aliases, a)
		}
		for _, r := range p.Uses {
			k := r.Reference + "\x00" + r.Target + "\x00" + strconv.FormatUint(uint64(r.StartByte), 10)
			if seenRel[k] {
				continue
			}
			seenRel[k] = true
			out.Uses = append(out.Uses, r)
		}
	}
	return out
}
