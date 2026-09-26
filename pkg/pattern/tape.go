package pattern

import (
	"fmt"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"path/filepath"
	"sort"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/tape"
)

// BuildTape attributes relPath via VM PackQueries, parses the host language,
// builds the structural tape (host leaves + embed islands spliced in), and
// sets TokenClass from paint actions (as-keyword, as-string, …).
func BuildTape(sess *project.Session, vm *LispVM, source []byte, relPath string) (cells []tape.Cell, lang string, err error) {
	if sess == nil {
		return nil, "", ingest.ErrNilSession
	}
	if vm == nil {
		return nil, "", fmt.Errorf("%w: walker: nil LispVM", ErrExtract)
	}
	rel := filepath.ToSlash(relPath)
	lang, ok, err := vm.AttributeHost(rel)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", fmt.Errorf("%w for %s (no path rule claims this file)", ingest.ErrUnsupportedLanguage, relPath)
	}
	pf, err := ingestutil.ParseSource(sess.Engine(), source, relPath, vm.GrammarForLanguage(lang))
	if err != nil {
		return nil, lang, err
	}
	defer pf.Close()
	cells = tape.Build(pf.Root, source, tape.WithAtomicTexts(vm.AtomicSpans(lang)), nil)
	cells = spliceEmbedCells(sess, cells, source, relPath, pf.Root, vm.Packs().Program())
	prog := vm.Packs().Program()
	paint := PaintClasses(prog, sess, pf.Root, source, rel)
	for i := range cells {
		cells[i].TokenClass = classForCell(paint, cells[i].StartByte, cells[i].EndByte, cells[i].Type, lang, prog)
	}
	return cells, lang, nil
}

// spliceEmbedCells replaces host leaves that cover an embed region with the
// embed language's tape cells (host-absolute spans). SPEC.md Regions: embeds are not skipped.
func spliceEmbedCells(sess *project.Session, cells []tape.Cell, source []byte, relPath string, hostRoot *sitter.Node, p *ExtractProgram) []tape.Cell {
	if p == nil || hostRoot == nil || len(source) == 0 {
		return cells
	}
	rel := stringsTrimDotSlash(filepathToSlash(relPath))
	loci := uniqueEmbedRegions(sess, p, rel, relPath, source, hostRoot, nil)
	if len(loci) == 0 {
		return cells
	}
	defer func() {
		for _, e := range loci {
			e.close()
		}
	}()

	// Process outer→inner is fine; each locus replaces overlapping host cells.
	// Sort by start then longer first so outer regions go first.
	sort.SliceStable(loci, func(i, j int) bool {
		if loci[i].base != loci[j].base {
			return loci[i].base < loci[j].base
		}
		return len(loci[i].content) > len(loci[j].content)
	})

	for _, emb := range loci {
		end := emb.base + uint32(len(emb.content))
		embCells := tape.Build(emb.root, emb.content, tape.WithAtomicTexts(p.atomicTexts(emb.lang)), nil)
		for i := range embCells {
			embCells[i].StartByte += emb.base
			embCells[i].EndByte += emb.base
		}
		cells = replaceSpanWithCells(cells, emb.base, end, embCells)
	}
	return cells
}

// replaceSpanWithCells drops host cells that overlap [start,end) and inserts
// repl (already host-absolute) in source order.
func replaceSpanWithCells(cells []tape.Cell, start, end uint32, repl []tape.Cell) []tape.Cell {
	if start >= end {
		return cells
	}
	out := make([]tape.Cell, 0, len(cells)+len(repl))
	inserted := false
	for _, c := range cells {
		// fully before
		if c.EndByte <= start {
			out = append(out, c)
			continue
		}
		// fully after
		if c.StartByte >= end {
			if !inserted {
				out = append(out, repl...)
				inserted = true
			}
			out = append(out, c)
			continue
		}
		// overlaps embed span → drop host cell (replaced by embed leaves)
	}
	if !inserted {
		out = append(out, repl...)
	}
	return out
}
