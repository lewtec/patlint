// Package reporoot finds the repository root for a path.
//
// v1 recognizes a Git checkout (directory or file .git, including linked
// worktrees). Additional pivots can be added later without changing the
// Find signature.
package reporoot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	lewpath "github.com/lewtec/lewkit/x/path"
)

// ErrNotFound is returned when no repository pivot is found above start.
var ErrNotFound = errors.New("repository root not found")

// Find returns the absolute path of the repository root that contains start.
// start may be a file or directory; the walk begins at its directory if it is
// a file, or at start itself if it is a directory (or does not exist yet).
//
// When no pivot is found, Find returns ErrNotFound.
func Find(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("reporoot: %w", err)
	}

	dir := abs
	if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
		dir = filepath.Dir(abs)
	}

	for {
		if hasGitPivot(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w: %s", ErrNotFound, abs)
		}
		dir = parent
	}
}

// hasGitPivot reports a Git checkout root: .git directory or gitdir file
// (linked worktree / submodule).
func hasGitPivot(dir string) bool {
	_, err := os.Stat(lewpath.New(dir, ".git").String())
	return err == nil
}
