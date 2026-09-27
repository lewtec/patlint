package apply

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
)

var (
	// ErrNoMatches means the overlay has nothing to write.
	ErrNoMatches = errors.New("no matches")
	// ErrCancelled means the user declined an interactive apply.
	ErrCancelled = errors.New("cancelled")
)

// Committer previews a session overlay and writes it back.
// StandardError and Input are the prompt streams. DryRun prints and does not write.
type Committer struct {
	StandardError io.Writer
	Input         io.Reader
	Interactive   bool
	DryRun        bool
	Backup        bool
}

// Session commits the overlay on session.
// An empty overlay returns ErrNoMatches.
func (committer Committer) Session(ctx context.Context, session *project.Session) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	if session == nil {
		return ingest.ErrNilSession
	}
	writes := project.OverlayWrites(session.FS)
	renames := project.OverlayRenames(session.FS)
	if len(writes) == 0 && len(renames) == 0 {
		return ErrNoMatches
	}
	edits := editsFromWrites(session.FS, writes)
	if committer.Interactive || committer.DryRun {
		if err := printFileMoves(committer.StandardError, renames); err != nil {
			return err
		}
		if err := printEditPlan(committer.StandardError, edits); err != nil {
			return err
		}
		if committer.DryRun {
			return nil
		}
		accepted, err := confirmApply(committer.StandardError, committer.Input)
		if err != nil {
			return err
		}
		if !accepted {
			return ErrCancelled
		}
	}
	if committer.Backup {
		if err := createBackups(ctx, session.Root, edits); err != nil {
			return err
		}
	}
	return ingest.WriteBack(ctx, session)
}

// Edits prints the edit list, or commits it onto root when DryRun is false.
func (committer Committer) Edits(ctx context.Context, root string, edits []project.Edit) error {
	if committer.DryRun {
		return printEditPlan(committer.StandardError, edits)
	}
	session, err := sessionFromEdits(root, edits)
	if err != nil {
		return err
	}
	return committer.Session(ctx, session)
}

func sessionFromEdits(root string, edits []project.Edit) (*project.Session, error) {
	session := project.NewSession(root)
	byFile := map[string][]project.Edit{}
	for _, edit := range edits {
		byFile[edit.File] = append(byFile[edit.File], edit)
	}
	writes := map[string][]byte{}
	for name, fileEdits := range byFile {
		relative := filepath.ToSlash(name)
		before, err := fs.ReadFile(session.FS, relative)
		if err != nil {
			return nil, err
		}
		after := project.ApplyEditsInMemory(before, fileEdits)
		if string(after) != string(before) {
			writes[relative] = after
		}
	}
	if len(writes) == 0 {
		return session, nil
	}
	return session.WithFS(project.NewPatchFS(session.FS, writes)), nil
}

func editsFromWrites(filesystem fs.FS, writes map[string][]byte) []project.Edit {
	var edits []project.Edit
	for name, after := range writes {
		before, err := lewpath.New(name).ReadFile(filesystem)
		if err != nil {
			edits = append(edits, project.Edit{File: name, NewText: string(after)})
			continue
		}
		edits = append(edits, project.EditContentDiff(name, before, after)...)
	}
	return edits
}

func printEditPlan(writer io.Writer, edits []project.Edit) error {
	if _, err := fmt.Fprintf(writer, "Edit plan (%d edits):\n", len(edits)); err != nil {
		return err
	}
	for _, edit := range edits {
		if _, err := fmt.Fprintf(writer, "  %s [%d:%d] → %q\n", edit.File, edit.StartByte, edit.EndByte, edit.NewText); err != nil {
			return err
		}
	}
	return nil
}

func printFileMoves(writer io.Writer, renames map[string]string) error {
	if len(renames) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(writer, "File moves (%d):\n", len(renames)); err != nil {
		return err
	}
	names := make([]string, 0, len(renames))
	for from := range renames {
		names = append(names, from)
	}
	slices.Sort(names)
	for _, from := range names {
		if _, err := fmt.Fprintf(writer, "  %s → %s\n", from, renames[from]); err != nil {
			return err
		}
	}
	return nil
}

func confirmApply(writer io.Writer, reader io.Reader) (bool, error) {
	if _, err := fmt.Fprint(writer, "Apply? [y/N] "); err != nil {
		return false, err
	}
	var answer string
	if _, err := fmt.Fscan(reader, &answer); err != nil {
		return false, err
	}
	return answer == "y" || answer == "Y", nil
}

func createBackups(ctx context.Context, directory string, edits []project.Edit) error {
	root, err := lewpath.Open(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	seen := map[string]bool{}
	for _, edit := range edits {
		if err := ctx.Err(); err != nil {
			return err
		}
		if seen[edit.File] {
			continue
		}
		seen[edit.File] = true
		name := filepath.ToSlash(edit.File)
		source, err := lewpath.New(name).ReadFile(root)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return err
		}
		if err := lewpath.New(name+".bak").WriteFile(root, source, 0o644); err != nil {
			return err
		}
	}
	return nil
}
