package project

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/lewtec/lewkit/x/io/atomic"
	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingestutil"
)

// ErrEditOutOfBounds is a span past the current file length.
var ErrEditOutOfBounds = errors.New("edit out of bounds")

// ApplyEditsInMemory applies edits to content using descending start offsets
// (same order as ApplyEdits). Invalid spans are skipped.
func ApplyEditsInMemory(content []byte, edits []Edit) []byte {
	if len(edits) == 0 {
		return content
	}
	sorted := append([]Edit(nil), edits...)
	slices.SortFunc(sorted, func(a, b Edit) int {
		return cmp.Compare(b.StartByte, a.StartByte)
	})
	buf := append([]byte(nil), content...)
	for _, e := range sorted {
		if int(e.EndByte) > len(buf) || e.StartByte > e.EndByte {
			continue
		}
		buf = append(buf[:e.StartByte], append([]byte(e.NewText), buf[e.EndByte:]...)...)
	}
	return buf
}

// EditContentDiff builds a single edit that turns before into after by replacing
// the minimal mid-span that differs (common prefix/suffix). If equal, returns nil.
func EditContentDiff(file string, before, after []byte) []Edit {
	if string(before) == string(after) {
		return nil
	}
	i := 0
	for i < len(before) && i < len(after) && before[i] == after[i] {
		i++
	}
	ja, jb := len(before), len(after)
	for ja > i && jb > i && before[ja-1] == after[jb-1] {
		ja--
		jb--
	}
	return []Edit{{
		File:    strings.TrimPrefix(file, "./"),
		Span:    ingestutil.Span{StartByte: uint32(i), EndByte: uint32(ja)},
		NewText: string(after[i:jb]),
	}}
}

// ApplyEdits applies text replacements to files under dir.
// Edits are grouped by file and applied from last to first byte offset
// so that earlier offsets remain valid. Cancel is observed between files.
func ApplyEdits(ctx context.Context, dir string, edits []Edit) error {
	byFile := map[string][]Edit{}
	for _, e := range edits {
		byFile[e.File] = append(byFile[e.File], e)
	}

	for file, fileEdits := range byFile {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := lewpath.New(dir, file).String()
		content, err := os.ReadFile(target)
		creating := false
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				hasCreate := false
				for _, e := range fileEdits {
					if e.StartByte == 0 && e.EndByte == 0 {
						hasCreate = true
						break
					}
				}
				if !hasCreate {
					return fmt.Errorf("reading %s: %w", file, err)
				}
				content = []byte{}
				creating = true
				if mkerr := os.MkdirAll(lewpath.New(target).Parent().String(), 0o755); mkerr != nil {
					return fmt.Errorf("mkdir for %s: %w", file, mkerr)
				}
			} else {
				return fmt.Errorf("reading %s: %w", file, err)
			}
		}

		applyOne := func(e Edit) error {
			if int(e.EndByte) > len(content) {
				return fmt.Errorf("%w in %s: end %d > len %d", ErrEditOutOfBounds, file, e.EndByte, len(content))
			}
			content = append(content[:e.StartByte], append([]byte(e.NewText), content[e.EndByte:]...)...)
			return nil
		}

		if creating || len(content) == 0 {
			var creates, rest []Edit
			for _, e := range fileEdits {
				if e.StartByte == 0 && e.EndByte == 0 {
					creates = append(creates, e)
				} else {
					rest = append(rest, e)
				}
			}
			for _, e := range creates {
				if err := applyOne(e); err != nil {
					return err
				}
			}
			slices.SortFunc(rest, func(a, b Edit) int {
				return cmp.Compare(b.StartByte, a.StartByte)
			})
			for _, e := range rest {
				if err := applyOne(e); err != nil {
					return err
				}
			}
		} else {
			slices.SortFunc(fileEdits, func(a, b Edit) int {
				return cmp.Compare(b.StartByte, a.StartByte)
			})
			for _, e := range fileEdits {
				if err := applyOne(e); err != nil {
					return err
				}
			}
		}

		if err := atomic.WriteFileFunction(target, func(w io.Writer) error {
			_, err := w.Write(content)
			return err
		}); err != nil {
			return fmt.Errorf("writing %s: %w", file, err)
		}

		if len(content) == 0 {
			if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("remove empty %s: %w", file, err)
			}
		}
	}

	return nil
}
