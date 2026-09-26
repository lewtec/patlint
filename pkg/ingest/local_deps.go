package ingest

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// LocalDepOpts controls LocalDepsInDeclSpan filtering.
type LocalDepOpts struct {
	// TopLevelOnly skips atom names that contain "." (Class.method).
	TopLevelOnly bool
}

// LocalDepsInDeclSpan returns names of same-file atoms referenced by uses that
// fall inside decl's remove span, excluding the moved source itself.
// Used by JS/Python residual import generation after ExtractDecl.
func LocalDepsInDeclSpan(result *project.Result, src Reference, decl DeclExtract, opts LocalDepOpts) []string {
	if result == nil {
		return nil
	}
	srcRef := src.String()
	srcPath := src.Path
	localEntities := map[string]bool{}
	for _, ent := range result.Atoms {
		ref := ParseReference(ent.Reference)
		if ref.Path != srcPath || ent.Reference == srcRef {
			continue
		}
		if ref.Name == "" {
			continue
		}
		if opts.TopLevelOnly && strings.Contains(ref.Name, ".") {
			continue
		}
		localEntities[ref.Name] = true
	}
	if len(localEntities) == 0 {
		return nil
	}
	var deps []string
	seen := map[string]bool{}
	for _, rel := range result.Uses {
		ref := ParseReference(rel.Reference)
		if ref.Path != srcPath {
			continue
		}
		if rel.StartByte < decl.RemoveStart || rel.EndByte > decl.RemoveEnd {
			continue
		}
		targetRef := ParseReference(rel.Target)
		if targetRef.Path != srcPath {
			continue
		}
		sym := targetRef.Name
		if sym == "" || seen[sym] || sym == src.Name {
			continue
		}
		if opts.TopLevelOnly && strings.Contains(sym, ".") {
			continue
		}
		if localEntities[sym] {
			seen[sym] = true
			deps = append(deps, sym)
		}
	}
	return deps
}

// InsertAt returns a pure insertion edit at pos (zero-width span).
func InsertAt(file string, pos uint32, text string) project.Edit {
	if text == "" {
		return project.Edit{}
	}
	return project.Edit{
		File:    strings.TrimPrefix(file, "./"),
		Span:    ingestutil.Span{StartByte: pos, EndByte: pos},
		NewText: text,
	}
}

// InsertLinesAt inserts missing lines (already filtered) as a single edit at pos.
// Returns nil when lines is empty.
func InsertLinesAt(file string, pos uint32, lines []string) []project.Edit {
	block := ingestutil.JoinLinesNL(lines)
	if block == "" {
		return nil
	}
	return []project.Edit{InsertAt(file, pos, block)}
}
