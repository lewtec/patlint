package ingestgo

import (
	"github.com/lewtec/patlint/pkg/project"
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
)

// findPathSegmentOccurrencesInStrings replaces oldPath with newPath only
// inside string literals and only on import-path segment boundaries.
func findPathSegmentOccurrencesInStrings(file string, content []byte, oldPath, newPath string) []project.Edit {
	return findInStringLiterals(file, content, oldPath, newPath, "")
}

// findPathSegmentOccurrencesInStringsWithParent replaces a path leaf only when
// the immediately preceding path prefix equals parentDir in full (not just the
// parent leaf). Using only the final parent component falsely rewrites other
// packages that share a basename path like cmd/.../driver/wallpaper vs
// pkg/.../driver/wallpaper.
func findPathSegmentOccurrencesInStringsWithParent(file string, content []byte, leaf, newLeaf, parentDir string) []project.Edit {
	return findInStringLiterals(file, content, leaf, newLeaf, parentDir)
}

func findInStringLiterals(file string, content []byte, oldBase, newBase, parentDir string) []project.Edit {
	if oldBase == "" || oldBase == newBase {
		return nil
	}
	var edits []project.Edit
	ingestutil.ForEachStringLiteral(content, func(seg string, start int) bool {
		sOff := 0
		for {
			idx := strings.Index(seg[sOff:], oldBase)
			if idx < 0 {
				break
			}
			posInSeg := sOff + idx
			pos := start + posInSeg
			endPos := pos + len(oldBase)
			keep := pathSegmentBounded(seg, posInSeg, posInSeg+len(oldBase))
			if keep && parentDir != "" {
				keep = pathSegmentHasParent(seg, posInSeg, parentDir)
			}
			if keep {
				edits = append(edits, project.Edit{
					File:    file,
					Span:    ingestutil.Span{StartByte: uint32(pos), EndByte: uint32(endPos)},
					NewText: newBase,
				})
			}
			sOff = posInSeg + len(oldBase)
		}
		return true
	})
	return edits
}

func pathSegmentBounded(seg string, start, end int) bool {
	if start > 0 && isPathSegmentChar(seg[start-1]) {
		return false
	}
	if end < len(seg) && isPathSegmentChar(seg[end]) {
		return false
	}
	return true
}

// pathSegmentHasParent reports whether the path immediately before leafStart is
// parentPath (multi-segment allowed), bounded on path-segment boundaries.
func pathSegmentHasParent(seg string, leafStart int, parentPath string) bool {
	if parentPath == "" {
		return leafStart == 1
	}
	if leafStart < 2 || seg[leafStart-1] != '/' {
		return false
	}
	parentStart := leafStart - 1 - len(parentPath)
	if parentStart < 1 {
		return false
	}
	if seg[parentStart:leafStart-1] != parentPath {
		return false
	}
	return pathSegmentBounded(seg, parentStart, leafStart-1)
}

func isPathSegmentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') ||
		b == '_' || b == '-' || b == '.' || b == '~' || b == '+'
}
