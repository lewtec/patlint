package ignore

import "strings"

// IsSkippedDirName reports dependency/build directory basenames that are part
// of the built-in rule set (and used while collecting .gitattributes so we do
// not scan node_modules for attributes files).
func IsSkippedDirName(name string) bool {
	switch strings.ToLower(name) {
	case "node_modules", ".git", "vendor", "dist", "build", "out", "coverage",
		".svelte-kit", ".astro", ".output", ".next", ".nuxt", ".venv", "venv", "__pycache__",
		"target", ".turbo", ".cache", ".parcel-cache":
		return true
	default:
		return false
	}
}

// DefaultSkippedDirNames is the built-in directory name list.
func DefaultSkippedDirNames() []string {
	return []string{
		"node_modules", ".git", "vendor", "dist", "build", "out", "coverage",
		".svelte-kit", ".astro", ".output", ".next", ".nuxt", ".venv", "venv", "__pycache__",
		"target", ".turbo", ".cache", ".parcel-cache",
	}
}
