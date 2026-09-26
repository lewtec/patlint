package project

import "github.com/lewtec/patlint/pkg/ingestutil"

// Edit describes a text replacement in a source file.
type Edit struct {
	File string
	ingestutil.Span
	NewText string
}

// DirMove is a filesystem directory rename relative to the project root.
type DirMove struct {
	From string
	To   string
}

// FileMove is a single-file rename (existing file → dest whose parent exists).
type FileMove struct {
	From string
	To   string
}

// Plan is directory/file renames plus text edits.
type Plan struct {
	DirMoves  []DirMove
	FileMoves []FileMove
	Edits     []Edit
}

// Empty reports whether the plan has no moves and no text edits.
func (p Plan) Empty() bool {
	return len(p.DirMoves) == 0 && len(p.FileMoves) == 0 && len(p.Edits) == 0
}
