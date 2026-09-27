package tape

import (
	"bytes"
	"slices"

	"github.com/lewtec/patlint/pkg/sitter"
)

// Leaves walks root and returns significant grammar leaves under pol.
// It does not attach targets, drop space-only spans, or resolve overlapping
// synthetic cells — use Build for the full tape.
//
// Emission order is document order (start-byte ascending). Emitted cells are
// non-nested (either true leaves or atomic subtrees), so preferInnermost is
// not required until synthetic target cells are merged.
func Leaves(root *sitter.Node, source []byte, pol Policy) []Cell {
	if root == nil || root.IsNull() {
		return nil
	}
	// Heuristic capacity: significant leaves are far fewer than source bytes.
	capHint := len(source) / 16
	if capHint < 32 {
		capHint = 32
	}
	out := make([]Cell, 0, capHint)
	collectLeaves(root, source, pol, &out)
	return out
}

// Build returns the finalized structural tape: grammar leaves, optional
// hyperlink targets by span, space dropped, innermost preferred, sorted and
// deduped. targets may be nil.
func Build(root *sitter.Node, source []byte, pol Policy, targets map[Span]string) []Cell {
	return FinishLeaves(Leaves(root, source, pol), source, targets)
}

// FinishLeaves is Build's post-pass on already-collected leaf cells.
func FinishLeaves(cells []Cell, source []byte, targets map[Span]string) []Cell {
	if len(targets) == 0 {
		return finalizeOrderedLeaves(cells, source)
	}
	return Finalize(cells, source, targets)
}

// Finalize merges targets into cells (and synthetic target-only spans), drops
// empty/space cells, prefers innermost spans, sorts by position, and dedupes.
// Finalize may reuse and overwrite the cells backing array; callers must not
// use cells after the call.
//
// Prefer Build when starting from a tree: it skips preferInnermost when targets
// is empty because Leaves already emits non-nested document-ordered cells.
func Finalize(cells []Cell, source []byte, targets map[Span]string) []Cell {
	if len(targets) > 0 {
		cells = attachTargets(cells, source, targets)
	}
	cells = filterSpace(cells, source)
	if len(cells) == 0 {
		return nil
	}
	// Always collapse nested spans for the public Finalize API (tests and
	// synthetic cell lists). Build() uses finalizeOrderedLeaves when safe.
	cells = preferInnermost(cells)
	return dedupeSpans(cells)
}

// finalizeOrderedLeaves is the fast path for Leaves output with no targets:
// space filter + equal-span dedupe only (no O(n log n) innermost pass).
func finalizeOrderedLeaves(cells []Cell, source []byte) []Cell {
	cells = filterSpace(cells, source)
	if len(cells) == 0 {
		return nil
	}
	return dedupeSpans(cells)
}

func filterSpace(cells []Cell, source []byte) []Cell {
	n := 0
	for _, a := range cells {
		if a.StartByte >= a.EndByte || isSpaceSpan(source, a) {
			continue
		}
		cells[n] = a
		n++
	}
	return cells[:n]
}

func dedupeSpans(cells []Cell) []Cell {
	if len(cells) == 0 {
		return nil
	}
	w := 1
	for i := 1; i < len(cells); i++ {
		prev, cur := &cells[w-1], cells[i]
		if prev.StartByte == cur.StartByte && prev.EndByte == cur.EndByte {
			if prev.Target == "" {
				prev.Target = cur.Target
			}
			if prev.Type == "" {
				prev.Type = cur.Type
			}
			continue
		}
		cells[w] = cur
		w++
	}
	return cells[:w]
}

// attachTargets sets Target on matching cells and appends synthetic cells for
// target spans that are not already leaves. O(cells + targets).
func attachTargets(cells []Cell, source []byte, targets map[Span]string) []Cell {
	if len(targets) == 0 {
		return cells
	}
	// First pass: attach known leaf spans.
	for i := range cells {
		if t, ok := targets[cells[i].Span]; ok {
			cells[i].Target = t
		}
	}
	// Second pass: synthetic target-only spans not present as leaves.
	// Index existing spans so we do not re-scan cells per target.
	have := make(map[Span]struct{}, len(cells))
	for _, c := range cells {
		have[c.Span] = struct{}{}
	}
	for s, target := range targets {
		if _, ok := have[s]; ok {
			continue
		}
		if int(s.EndByte) > len(source) || s.StartByte >= s.EndByte {
			continue
		}
		cells = append(cells, Cell{
			Span:   s,
			Target: target,
		})
		have[s] = struct{}{}
	}
	return cells
}

// preferInnermost drops spans that strictly contain another candidate, keeping
// the nested leaves. Sorted sweep with a stack — O(n log n).
// Mutates and reuses cands' backing array.
func preferInnermost(cands []Cell) []Cell {
	if len(cands) <= 1 {
		return cands
	}
	slices.SortStableFunc(cands, func(a, b Cell) int {
		if a.StartByte != b.StartByte {
			if a.StartByte < b.StartByte {
				return -1
			}
			return 1
		}
		// Longer first at the same start so outers are seen before inners.
		if a.EndByte != b.EndByte {
			if a.EndByte > b.EndByte {
				return -1
			}
			return 1
		}
		return 0
	})
	drop := make([]bool, len(cands))
	stack := make([]int, 0, 16)
	for i := range cands {
		c := cands[i]
		for len(stack) > 0 && cands[stack[len(stack)-1]].EndByte <= c.StartByte {
			stack = stack[:len(stack)-1]
		}
		for len(stack) > 0 {
			j := stack[len(stack)-1]
			o := cands[j]
			// o strictly contains c → drop outer o
			if o.StartByte <= c.StartByte && o.EndByte >= c.EndByte &&
				(o.StartByte < c.StartByte || o.EndByte > c.EndByte) {
				drop[j] = true
				stack = stack[:len(stack)-1]
				continue
			}
			break
		}
		stack = append(stack, i)
	}
	// Compact kept cells into the same slice.
	w := 0
	for i, c := range cands {
		if !drop[i] {
			cands[w] = c
			w++
		}
	}
	cands = cands[:w]
	// Restore source order (start asc, end asc) for stable tape consumers.
	slices.SortStableFunc(cands, func(a, b Cell) int {
		if a.StartByte != b.StartByte {
			if a.StartByte < b.StartByte {
				return -1
			}
			return 1
		}
		if a.EndByte != b.EndByte {
			if a.EndByte < b.EndByte {
				return -1
			}
			return 1
		}
		return 0
	})
	return cands
}

// TapeVisit is the tape decision at n. emit writes a cell; muteKids means
// children must not also become tape cells (atomic / true leaf). Index walks
// still descend. Child is a pointer into the snapshot (no parser lock).
func (p Policy) TapeVisit(n *sitter.Node, source []byte) (cell Cell, emit, muteKids bool) {
	if n == nil || n.IsNull() {
		return Cell{}, false, true
	}
	start, end := n.StartByte(), n.EndByte()
	if start >= end {
		return Cell{}, false, true
	}
	cc := n.ChildCount()
	if cc == 0 {
		if isSpaceBytes(source, start, end) {
			return Cell{}, false, true
		}
		return Cell{Span: Span{StartByte: start, EndByte: end}, Type: n.Type()}, true, true
	}
	if p.AtomicSpan != nil && p.atomicSpan(source, start, end) {
		return Cell{Span: Span{StartByte: start, EndByte: end}, Type: n.Type()}, true, true
	}
	if p.usesDefaultAtomicTypes() {
		if looksLikeQuotedSpan(source, start, end) {
			typ := n.Type()
			if p.atomicType(typ) {
				return Cell{Span: Span{StartByte: start, EndByte: end}, Type: typ}, true, true
			}
		}
	} else {
		typ := n.Type()
		if p.atomicType(typ) {
			return Cell{Span: Span{StartByte: start, EndByte: end}, Type: typ}, true, true
		}
	}
	return Cell{}, false, false
}

func collectLeaves(n *sitter.Node, source []byte, pol Policy, out *[]Cell) {
	cell, emit, muteKids := pol.TapeVisit(n, source)
	if emit {
		*out = append(*out, cell)
	}
	if muteKids {
		return
	}
	cc := n.ChildCount()
	for i := uint32(0); i < cc; i++ {
		collectLeaves(n.Child(i), source, pol, out)
	}
}

func isSpaceSpan(src []byte, c Cell) bool {
	return isSpaceBytes(src, c.StartByte, c.EndByte)
}

func isSpaceBytes(src []byte, start, end uint32) bool {
	if start >= end || int(end) > len(src) {
		return false
	}
	b := src[start:end]
	return len(b) > 0 && len(bytes.TrimSpace(b)) == 0
}

// looksLikeQuotedSpan is a cheap gate before Type() for default string atomics.
func looksLikeQuotedSpan(src []byte, start, end uint32) bool {
	if end < start+2 || int(end) > len(src) {
		return false
	}
	q := src[start]
	return (q == '"' || q == '\'' || q == '`') && src[end-1] == q
}
