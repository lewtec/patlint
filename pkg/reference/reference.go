package reference

import "strings"

// Reference is a parsed provider:path::symbol string.
type Reference struct {
	Provider string // e.g. "path", "go", "python", "node"
	Path     string // e.g. "./main.go", "fmt"
	Name     string // e.g. "main", "Println"; empty for file/package refs
}

// Parse splits a "provider:path::symbol" string.
func Parse(s string) Reference {
	var r Reference

	base := s
	if j := strings.Index(base, "::"); j >= 0 {
		r.Name = base[j+2:]
		base = base[:j]
	}

	if i := strings.Index(base, ":"); i >= 0 {
		r.Provider = strings.ToLower(base[:i])
		r.Path = base[i+1:]
		return r
	}

	// Shorthand reference: <path>::<symbol> or <path>
	if r.Name != "" || strings.Contains(base, "/") || strings.HasPrefix(base, ".") {
		r.Provider = "path"
		if strings.HasPrefix(base, "./") || strings.HasPrefix(base, "../") || strings.HasPrefix(base, "/") {
			r.Path = base
		} else {
			r.Path = "./" + base
		}
		return r
	}

	// Bare identifier (kept as-is).
	r.Path = base
	return r
}

// String formats the reference back to its canonical form.
func (r Reference) String() string {
	base := r.Path
	if r.Provider != "" {
		base = strings.ToLower(r.Provider) + ":" + r.Path
	}
	if r.Name != "" {
		return base + "::" + r.Name
	}
	return base
}

// FileRef builds a path-provider file reference: path:./file
func FileRef(path string) string {
	return "path:" + path
}

// AtomRef builds a path-provider symbol reference: path:./file::symbol
func AtomRef(path, symbol string) string {
	return "path:" + path + "::" + symbol
}

// NormalizePathReference normalizes path-provider references so relative paths
// are consistently prefixed.
func NormalizePathReference(ref Reference) Reference {
	if strings.ToLower(ref.Provider) != "path" {
		return ref
	}
	ref.Provider = "path"
	if ref.Path == "" || ref.Path == "." {
		ref.Path = "./"
		return ref
	}
	if strings.HasPrefix(ref.Path, "./") || strings.HasPrefix(ref.Path, "../") || strings.HasPrefix(ref.Path, "/") {
		return ref
	}
	ref.Path = "./" + ref.Path
	return ref
}
