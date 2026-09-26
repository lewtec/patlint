package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/lewtec/lewkit/x/io/atomic"
	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
)

// applyEditPlanOptions controls plan preview and write behavior for a full
// edit list (mv; rewrite -i/-n path). Stream apply reuses backup/ensure only.
type applyEditPlanOptions struct {
	Interactive bool
	DryRun      bool
	Backup      bool
}

// applyEditPlan prints the plan when Interactive or DryRun, optionally asks
// for confirmation, then backups / creates missing files / ApplyEdits.
// DryRun never writes. Empty edits is a no-op success (callers may error first).
func applyEditPlan(ctx context.Context, errw io.Writer, in io.Reader, root string, edits []project.Edit, opts applyEditPlanOptions) error {
	return applyMovePlan(ctx, errw, in, root, project.Plan{Edits: edits}, opts)
}

// commitOverlay is the rewrite/run commit: preview the transaction, then WriteBack.
func commitOverlay(ctx context.Context, errw io.Writer, in io.Reader, sess *project.Session, opts applyEditPlanOptions) error {
	if sess == nil {
		return ingest.ErrNilSession
	}
	writes := project.OverlayWrites(sess.FS)
	renames := project.OverlayRenames(sess.FS)
	if len(writes) == 0 && len(renames) == 0 {
		return ErrNoMatches
	}
	edits := overlayEdits(sess.Root, writes)
	if opts.Interactive || opts.DryRun {
		if err := printRenamePlan(errw, renames); err != nil {
			return err
		}
		if err := printEditPlan(errw, edits); err != nil {
			return err
		}
		if opts.DryRun {
			return nil
		}
		ok, err := confirmApply(errw, in)
		if err != nil {
			return err
		}
		if !ok {
			return ErrCancelled
		}
	}
	if opts.Backup {
		if err := createBackups(ctx, sess.Root, edits); err != nil {
			return err
		}
	}
	return ingest.WriteBack(ctx, sess)
}

func sessionFromEdits(root string, edits []project.Edit) (*project.Session, error) {
	sess := newSession(root)
	byFile := map[string][]project.Edit{}
	for _, e := range edits {
		byFile[e.File] = append(byFile[e.File], e)
	}
	writes := map[string][]byte{}
	for name, fileEdits := range byFile {
		rel := filepath.ToSlash(name)
		before, err := fs.ReadFile(sess.FS, rel)
		if err != nil {
			return nil, err
		}
		after := project.ApplyEditsInMemory(before, fileEdits)
		if string(after) != string(before) {
			writes[rel] = after
		}
	}
	if len(writes) == 0 {
		return sess, nil
	}
	return sess.WithFS(project.NewPatchFS(sess.FS, writes)), nil
}

func overlayEdits(root string, writes map[string][]byte) []project.Edit {
	var out []project.Edit
	for name, after := range writes {
		before, err := os.ReadFile(lewpath.New(root, filepath.FromSlash(name)).String())
		if err != nil {
			out = append(out, project.Edit{File: name, NewText: string(after)})
			continue
		}
		out = append(out, project.EditContentDiff(name, before, after)...)
	}
	return out
}

// applyMovePlan is applyEditPlan for a full Rename plan (DirMoves + Edits).
func applyMovePlan(ctx context.Context, errw io.Writer, in io.Reader, root string, plan project.Plan, opts applyEditPlanOptions) error {
	if plan.Empty() {
		slog.Debug("apply plan: empty")
		return nil
	}
	slog.Debug("apply plan",
		"root", root,
		"dir_moves", len(plan.DirMoves),
		"edits", len(plan.Edits),
		"files", editFileCount(plan.Edits),
		"interactive", opts.Interactive,
		"dry_run", opts.DryRun,
		"backup", opts.Backup,
	)
	if opts.Interactive || opts.DryRun {
		if err := printMovePlan(errw, plan); err != nil {
			return err
		}
		if opts.DryRun {
			slog.Debug("apply plan: dry-run, skip write")
			return nil
		}
		ok, err := confirmApply(errw, in)
		if err != nil {
			return err
		}
		if !ok {
			slog.Debug("apply plan: cancelled by user")
			return ErrCancelled
		}
	}
	return writeMovePlan(ctx, root, plan, opts.Backup)
}

func printMovePlan(w io.Writer, plan project.Plan) error {
	if len(plan.DirMoves) > 0 {
		if _, err := fmt.Fprintf(w, "Dir moves (%d):\n", len(plan.DirMoves)); err != nil {
			return err
		}
		for _, m := range plan.DirMoves {
			if _, err := fmt.Fprintf(w, "  %s → %s\n", m.From, m.To); err != nil {
				return err
			}
		}
	}
	return printEditPlan(w, plan.Edits)
}

func printRenamePlan(w io.Writer, renames map[string]string) error {
	if len(renames) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "File moves (%d):\n", len(renames)); err != nil {
		return err
	}
	names := make([]string, 0, len(renames))
	for from := range renames {
		names = append(names, from)
	}
	sort.Strings(names)
	for _, from := range names {
		if _, err := fmt.Fprintf(w, "  %s → %s\n", from, renames[from]); err != nil {
			return err
		}
	}
	return nil
}

// writeMovePlan optionally backups text-edit files, ensures create targets, then ApplyPlan.
// DirMoves run before ensureEditFiles so post-rename paths are not pre-created
// as empty files (that would block os.Rename of the package tree).
func writeMovePlan(ctx context.Context, root string, plan project.Plan, backup bool) error {
	slog.Debug("write plan", "root", root, "dir_moves", len(plan.DirMoves), "edits", len(plan.Edits), "backup", backup)
	if backup {
		if err := createBackups(ctx, root, plan.Edits); err != nil {
			return err
		}
	}
	// Apply directory renames first; then ensure/create only remaining edit targets.
	if err := ingest.ApplyPlan(ctx, root, project.Plan{DirMoves: plan.DirMoves}); err != nil {
		return err
	}
	if err := ensureEditFiles(ctx, root, plan.Edits); err != nil {
		return err
	}
	if err := project.ApplyEdits(ctx, root, plan.Edits); err != nil {
		return err
	}
	slog.Debug("write plan: done")
	return nil
}

// writeEdits optionally backups, ensures files exist, then ApplyEdits.
func writeEdits(ctx context.Context, root string, edits []project.Edit, backup bool) error {
	slog.Debug("write edits", "root", root, "edits", len(edits), "backup", backup)
	if backup {
		if err := createBackups(ctx, root, edits); err != nil {
			return err
		}
	}
	if err := ensureEditFiles(ctx, root, edits); err != nil {
		return err
	}
	if err := project.ApplyEdits(ctx, root, edits); err != nil {
		return err
	}
	slog.Debug("write edits: done")
	return nil
}

func editFileCount(edits []project.Edit) int {
	seen := map[string]bool{}
	for _, e := range edits {
		seen[e.File] = true
	}
	return len(seen)
}

func printEditPlan(w io.Writer, edits []project.Edit) error {
	if _, err := fmt.Fprintf(w, "Edit plan (%d edits):\n", len(edits)); err != nil {
		return err
	}
	for _, e := range edits {
		if _, err := fmt.Fprintf(w, "  %s [%d:%d] → %q\n", e.File, e.StartByte, e.EndByte, e.NewText); err != nil {
			return err
		}
	}
	return nil
}

func confirmApply(w io.Writer, r io.Reader) (bool, error) {
	if _, err := fmt.Fprint(w, "Apply? [y/N] "); err != nil {
		return false, err
	}
	var answer string
	if _, err := fmt.Fscan(r, &answer); err != nil {
		return false, err
	}
	return answer == "y" || answer == "Y", nil
}

func createBackups(ctx context.Context, dir string, edits []project.Edit) error {
	seen := map[string]bool{}
	for _, e := range edits {
		if err := ctx.Err(); err != nil {
			return err
		}
		if seen[e.File] {
			continue
		}
		seen[e.File] = true
		src := lewpath.New(dir, e.File).String()
		dst := src + ".bak"
		in, err := os.Open(src)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if cerr := in.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func ensureEditFiles(ctx context.Context, dir string, edits []project.Edit) error {
	seen := map[string]bool{}
	for _, e := range edits {
		if err := ctx.Err(); err != nil {
			return err
		}
		if seen[e.File] {
			continue
		}
		seen[e.File] = true

		p := lewpath.New(dir, e.File).String()
		_, err := os.Stat(p)
		if err == nil {
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(lewpath.New(p).Parent().String(), 0o755); err != nil {
			return err
		}
		if err := atomic.WriteString(p, ""); err != nil {
			return err
		}
	}
	return nil
}
