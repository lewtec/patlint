package ingest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/project"
)

// ApplyPlan applies directory renames then text edits under dir.
// DirMoves run first so package-tree renames (including untracked co-located
// files) complete before import/consumer rewrites target post-move paths.
func ApplyPlan(ctx context.Context, dir string, plan project.Plan) error {
	for _, m := range plan.DirMoves {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := applyDirectoryMove(dir, m); err != nil {
			return err
		}
	}
	for _, m := range plan.FileMoves {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := applyFileMove(dir, m); err != nil {
			return err
		}
	}
	return project.ApplyEdits(ctx, dir, plan.Edits)
}

func applyFileMove(root string, m project.FileMove) error {
	from := strings.TrimPrefix(filepath.ToSlash(m.From), "./")
	to := strings.TrimPrefix(filepath.ToSlash(m.To), "./")
	if from == "" || to == "" {
		return ErrDirMoveEmptyPaths
	}
	if from == to {
		return nil
	}
	fromAbs := lewpath.New(root, filepath.FromSlash(from)).String()
	toAbs := lewpath.New(root, filepath.FromSlash(to)).String()
	st, err := os.Stat(fromAbs)
	if err != nil {
		return fmt.Errorf("rename file %s: %w", from, err)
	}
	if st.IsDir() {
		return fmt.Errorf("rename file %s: is a directory", from)
	}
	if _, err := os.Stat(toAbs); err == nil {
		return fmt.Errorf("rename file %s → %s: dest exists", from, to)
	}
	parent := filepath.Dir(toAbs)
	pst, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("rename file %s → %s: dest parent: %w", from, to, err)
	}
	if !pst.IsDir() {
		return fmt.Errorf("rename file %s → %s: dest parent is not a dir", from, to)
	}
	if err := os.Rename(fromAbs, toAbs); err != nil {
		return fmt.Errorf("rename file %s → %s: %w", from, to, err)
	}
	slog.Debug("applyFileMove", "from", from, "to", to)
	return nil
}

// applyDirectoryMove renames From → To under root (slash paths). Creates parent of To
// when missing. Fails if From is missing or To already exists.
func applyDirectoryMove(root string, m project.DirMove) error {
	from := CleanRelDir(m.From)
	to := CleanRelDir(m.To)
	if from == "" || to == "" {
		return ErrDirMoveEmptyPaths
	}
	if from == to {
		return nil
	}
	fromAbs := lewpath.New(root, filepath.FromSlash(from)).String()
	toAbs := lewpath.New(root, filepath.FromSlash(to)).String()
	if err := os.MkdirAll(filepath.Dir(toAbs), 0o755); err != nil {
		return fmt.Errorf("mkdir parent for %s: %w", to, err)
	}
	if err := os.Rename(fromAbs, toAbs); err != nil {
		return fmt.Errorf("rename dir %s → %s: %w", from, to, err)
	}
	slog.Debug("applyDirectoryMove", "from", from, "to", to)
	return nil
}

// canDirectoryRename reports whether From can be os.Rename'd to To under root:
// From exists as a directory and To does not exist (no merge into an existing tree).
func canDirectoryRename(root, from, to string) bool {
	from = CleanRelDir(from)
	to = CleanRelDir(to)
	if from == "" || to == "" || from == to {
		return false
	}
	fromAbs := lewpath.New(root, filepath.FromSlash(from)).String()
	toAbs := lewpath.New(root, filepath.FromSlash(to)).String()
	fi, err := os.Stat(fromAbs)
	if err != nil || !fi.IsDir() {
		return false
	}
	if _, err := os.Stat(toAbs); err == nil {
		return false
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return true
}
