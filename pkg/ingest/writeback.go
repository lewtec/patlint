package ingest

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/project"
)

// WriteBack commits file Renames then Writes onto sess.Root. No MkdirAll.
// Directory Rename/Delete flatten is still an open hole.
func WriteBack(ctx context.Context, sess *project.Session) error {
	if sess == nil {
		return ErrNilSession
	}
	renames := project.OverlayRenames(sess.FS)
	order, err := fileRenameOrder(renames)
	if err != nil {
		return err
	}
	for _, from := range order {
		if err := ctx.Err(); err != nil {
			return err
		}
		to := renames[from]
		if err := applyFileMove(sess.Root, project.FileMove{From: from, To: to}); err != nil {
			return err
		}
	}
	writes := project.OverlayWrites(sess.FS)
	names := make([]string, 0, len(writes))
	for n := range writes {
		names = append(names, n)
	}
	sort.Strings(names)
	root, err := lewpath.Open(sess.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := lewpath.New(filepath.ToSlash(name)).WriteFile(root, writes[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// SessionFromPlan is a Write+file-Rename overlay. DirMoves are not first-cut.
func SessionFromPlan(ctx context.Context, sess *project.Session, plan project.Plan) (*project.Session, error) {
	if sess == nil {
		return nil, ErrNilSession
	}
	if len(plan.DirMoves) > 0 {
		return nil, fmt.Errorf("overlay: directory rename is not in the first cut")
	}
	parent := sess.FS
	if parent == nil {
		opened, err := lewpath.Open(sess.Root)
		if err != nil {
			return nil, err
		}
		parent = opened
	}
	renames := map[string]string{}
	for _, m := range plan.FileMoves {
		renames[m.From] = m.To
	}
	if len(renames) > 0 {
		parent = project.NewPatchFSLayer(parent, nil, renames)
	}
	byFile := map[string][]project.Edit{}
	for _, e := range plan.Edits {
		rel := filepath.ToSlash(strings.TrimPrefix(e.File, "./"))
		byFile[rel] = append(byFile[rel], e)
	}
	writes := map[string][]byte{}
	for rel, fileEdits := range byFile {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		before, err := fs.ReadFile(parent, rel)
		if err != nil {
			return nil, err
		}
		after := project.ApplyEditsInMemory(before, fileEdits)
		if string(after) != string(before) {
			writes[rel] = after
		}
	}
	if len(writes) == 0 && len(renames) == 0 {
		return sess, nil
	}
	return sess.WithFS(project.NewPatchFSLayer(parent, writes, nil)), nil
}

func fileRenameOrder(renames map[string]string) ([]string, error) {
	dest := map[string]bool{}
	for _, to := range renames {
		dest[to] = true
	}
	var order []string
	left := make(map[string]string, len(renames))
	for k, v := range renames {
		left[k] = v
	}
	for len(left) > 0 {
		progress := false
		for from, to := range left {
			if dest[from] {
				continue
			}
			order = append(order, from)
			delete(dest, to)
			delete(left, from)
			progress = true
		}
		if !progress {
			return nil, fmt.Errorf("rename cycle")
		}
	}
	return order, nil
}
