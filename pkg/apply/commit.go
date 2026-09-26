package apply

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
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

// Options controls preview and backup for a commit.
type Options struct {
	Interactive bool
	DryRun      bool
	Backup      bool
}

// Commit previews the session overlay, then WriteBack.
// DryRun prints and does not write. Empty overlay is ErrNoMatches.
func Commit(ctx context.Context, errw io.Writer, in io.Reader, sess *project.Session, opts Options) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
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
		if err := printRenames(errw, renames); err != nil {
			return err
		}
		if err := PrintEdits(errw, edits); err != nil {
			return err
		}
		if opts.DryRun {
			return nil
		}
		ok, err := confirm(errw, in)
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

// SessionFromEdits builds a session whose FS is the in-memory result of edits.
func SessionFromEdits(root string, edits []project.Edit) (*project.Session, error) {
	sess := project.NewSession(root)
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

// PrintEdits writes the edit list. Used by run --fix --dry-run.
func PrintEdits(w io.Writer, edits []project.Edit) error {
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

func printRenames(w io.Writer, renames map[string]string) error {
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
	slices.Sort(names)
	for _, from := range names {
		if _, err := fmt.Fprintf(w, "  %s → %s\n", from, renames[from]); err != nil {
			return err
		}
	}
	return nil
}

func confirm(w io.Writer, r io.Reader) (bool, error) {
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
