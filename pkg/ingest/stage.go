package ingest

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/projectfs"
)

// StageEdits applies edits in memory on top of base FS (nil = disk) under dir.
// Returns an overlay with staged file contents (absolute paths). Does not write disk.
func StageEdits(ctx context.Context, dir string, base projectfs.FS, edits []project.Edit) (*projectfs.Overlay, error) {
	if base == nil {
		base = projectfs.OS{}
	}
	rootAbs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	ov := projectfs.NewOverlay(base)

	byFile := map[string][]project.Edit{}
	for _, e := range edits {
		byFile[e.File] = append(byFile[e.File], e)
	}

	for file, fileEdits := range byFile {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rel := strings.TrimPrefix(filepath.ToSlash(file), "./")
		abs := lewpath.New(rootAbs, filepath.FromSlash(rel)).String()
		content, err := ov.ReadFile(abs)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				createAllowed := true
				for _, e := range fileEdits {
					if e.EndByte != 0 {
						createAllowed = false
						break
					}
				}
				if !createAllowed {
					return nil, fmt.Errorf("reading %s: %w", file, err)
				}
				content = []byte{}
			} else {
				return nil, fmt.Errorf("reading %s: %w", file, err)
			}
		}

		slices.SortFunc(fileEdits, func(a, b project.Edit) int {
			return cmp.Compare(b.StartByte, a.StartByte)
		})

		for _, e := range fileEdits {
			if int(e.EndByte) > len(content) {
				return nil, fmt.Errorf("%w in %s: end %d > len %d", ErrEditOutOfBounds, file, e.EndByte, len(content))
			}
			content = append(content[:e.StartByte], append([]byte(e.NewText), content[e.EndByte:]...)...)
		}
		ov.Set(abs, content)
	}
	return ov, nil
}

// CommitOverlay writes overlay entries that differ from base to disk.
// Used after a successful staged validation. Empty content removes the file.
func CommitOverlay(ctx context.Context, ov *projectfs.Overlay) error {
	if ov == nil {
		return nil
	}
	for _, abs := range ov.Paths() {
		if err := ctx.Err(); err != nil {
			return err
		}
		content, ok := ov.Get(abs)
		if !ok {
			continue
		}
		if len(content) == 0 {
			if err := os.Remove(abs); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("remove %s: %w", abs, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", abs, err)
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", abs, err)
		}
	}
	return nil
}

// ValidateStagedResult checks that entity identifier spans in result still match
// symbol leaves on fsys (basic post-edit gate). Walker.ValidateStaged loads.
func ValidateStagedResult(ctx context.Context, dir string, fsys projectfs.FS, result *project.Result) error {
	if result == nil {
		return fmt.Errorf("validate staged: nil Result")
	}
	if fsys == nil {
		fsys = projectfs.OS{}
	}
	rootAbs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	for _, ent := range result.Atoms {
		if err := ctx.Err(); err != nil {
			return err
		}
		ref := ParseReference(ent.Reference)
		if ref.Name == "" {
			continue
		}
		rel := strings.TrimPrefix(ref.Path, "./")
		abs := lewpath.New(rootAbs, filepath.FromSlash(rel)).String()
		data, err := fsys.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("staged missing %s: %w", rel, err)
		}
		if int(ent.EndByte) > len(data) || ent.StartByte > ent.EndByte {
			return fmt.Errorf("%w for %s", ErrStagedBadSpan, ent.Reference)
		}
		got := string(data[ent.StartByte:ent.EndByte])
		want := AtomName(ref.Name)
		if got != want {
			return fmt.Errorf("%w %s: %q != %q", ErrStagedEntityMismatch, ent.Reference, got, want)
		}
	}
	return nil
}
