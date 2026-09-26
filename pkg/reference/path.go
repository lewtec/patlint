package reference

import (
	"path/filepath"
	"strings"
)

// LastPathComponent returns the final segment of a slash-separated path.
// A single trailing slash is ignored so "pkg/foo/" and "pkg/foo" agree.
func LastPathComponent(s string) string {
	s = strings.TrimSuffix(s, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// JoinProviderPath joins slash-separated provider path segments, ignoring empty
// parts and trimming surrounding slashes from each argument.
func JoinProviderPath(base, name string) string {
	base = strings.Trim(base, "/")
	name = strings.Trim(name, "/")
	if base == "" {
		return name
	}
	if name == "" {
		return base
	}
	return base + "/" + name
}

// ParentProviderPath returns the parent slash-separated provider path, or "" at
// the root (single segment, empty, or ".").
func ParentProviderPath(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(path)))
	if parent == "." {
		return ""
	}
	return parent
}
