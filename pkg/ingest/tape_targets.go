package ingest

import (
	"path/filepath"
	"strings"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/store"
	"github.com/lewtec/patlint/pkg/tape"
)

// TapeUseTargets maps use-site byte spans in fileRel to resolved Target refs.
// Shared by pattern matching ((ref …) holes), abstract projection, and annotate
// hyperlinks so all join the structural tape the same way.
//
// Sources:
//   - Uses (qualified/bare name targets)
//   - Aliases with non-empty Target (import path spans → path:/go:/… refs)
//
// Spans with empty Target are omitted.
func TapeUseTargets(result *project.Result, fileRel string) map[tape.Span]string {
	if result == nil {
		return nil
	}
	norm := normTapePath(fileRel)
	out := map[tape.Span]string{}
	for _, use := range result.Uses {
		ref := ParseReference(use.Reference)
		if normTapePath(ref.Path) != norm || use.Target == "" || use.StartByte >= use.EndByte {
			continue
		}
		out[tape.Span{StartByte: use.StartByte, EndByte: use.EndByte}] = use.Target
	}
	// Import path tokens (e.g. Nix import ./foo.nix → @path:./foo.nix).
	// Prefer Alias spans over Uses when both exist on the same range.
	// Path token (ImportPathStart/End) is the leaf; Start/End may be the
	// whole binding (other = import ./foo.nix).
	for _, a := range result.Aliases {
		ref := ParseReference(a.Reference)
		if normTapePath(ref.Path) != norm || a.Target == "" {
			continue
		}
		start, end := a.StartByte, a.EndByte
		if a.ImportPathEnd > a.ImportPathStart {
			start, end = a.ImportPathStart, a.ImportPathEnd
		}
		if start >= end {
			continue
		}
		out[tape.Span{StartByte: start, EndByte: end}] = a.Target
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// TapeUseTargetsStore is TapeUseTargets on RelationBinds + RelationAlias.
func TapeUseTargetsStore(st *store.Store, fileRel string) map[tape.Span]string {
	if st == nil {
		return nil
	}
	norm := normTapePath(fileRel)
	out := map[tape.Span]string{}
	for _, t := range st.Rows(store.RelationBinds) {
		if len(t) < 5 || normTapePath(t[0]) != norm || t[4] == "" {
			continue
		}
		s, e := store.Atoi(t[1]), store.Atoi(t[2])
		if s >= e {
			continue
		}
		out[tape.Span{StartByte: s, EndByte: e}] = t[4]
	}
	for _, t := range st.Rows(store.RelationAlias) {
		if len(t) < 4 || t[3] == "" {
			continue
		}
		if normTapePath(ParseReference(t[0]).Path) != norm {
			continue
		}
		s, e := store.Atoi(t[1]), store.Atoi(t[2])
		if len(t) >= 7 {
			if ps, pe := store.Atoi(t[5]), store.Atoi(t[6]); pe > ps {
				s, e = ps, pe
			}
		}
		if s >= e {
			continue
		}
		out[tape.Span{StartByte: s, EndByte: e}] = t[3]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normTapePath(p string) string {
	return strings.TrimPrefix(filepath.ToSlash(p), "./")
}
