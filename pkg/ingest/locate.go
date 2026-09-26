package ingest

import (
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/store"
	"path/filepath"
	"strings"
	"unicode"
)

// Hit is a symbol under a byte offset (definition or usage).
type Hit struct {
	// Reference is the entity reference when known (usage Target or entity self-ref).
	Reference string
	StartByte uint32
	EndByte   uint32
	// IsDef is true when the hit is an entity definition span.
	IsDef bool
}

// HitAtByte finds the tightest entity or relation span in result that contains
// byteOff within fileRel (project-relative path with optional ./).
// Result is a store projection; HitAtByteStore reads RelationAtom / RelationBinds.
func HitAtByte(result *project.Result, fileRel string, byteOff int) (Hit, bool) {
	if result == nil || byteOff < 0 {
		return Hit{}, false
	}
	fileRel = strings.TrimPrefix(filepath.ToSlash(fileRel), "./")
	var best Hit
	bestSpan := -1

	consider := func(ref string, start, end uint32, isDef bool) {
		if int(start) > byteOff || int(end) <= byteOff {
			return
		}
		span := int(end - start)
		if bestSpan < 0 || span < bestSpan {
			best = Hit{Reference: ref, StartByte: start, EndByte: end, IsDef: isDef}
			bestSpan = span
		}
	}

	for _, ent := range result.Atoms {
		er := ParseReference(ent.Reference)
		rel := strings.TrimPrefix(filepath.ToSlash(er.Path), "./")
		if rel != fileRel {
			continue
		}
		consider(ent.Reference, ent.StartByte, ent.EndByte, true)
	}
	for _, reln := range result.Uses {
		// Use.Reference is often path::usage site; Target is the entity.
		rr := ParseReference(reln.Reference)
		rel := strings.TrimPrefix(filepath.ToSlash(rr.Path), "./")
		if rel != fileRel {
			// Some relations use importer path only in Reference differently; try Target path for def-side? no.
			continue
		}
		target := reln.Target
		if target == "" {
			target = reln.Reference
		}
		consider(target, reln.StartByte, reln.EndByte, false)
	}
	if bestSpan < 0 {
		return Hit{}, false
	}
	return best, true
}

// HitAtByteStore is HitAtByte on RelationAtom (defs) and RelationBinds (uses).
func HitAtByteStore(st *store.Store, fileRel string, byteOff int) (Hit, bool) {
	if st == nil || byteOff < 0 {
		return Hit{}, false
	}
	fileRel = strings.TrimPrefix(filepath.ToSlash(fileRel), "./")
	var best Hit
	bestSpan := -1
	consider := func(ref string, start, end uint32, isDef bool) {
		if int(start) > byteOff || int(end) <= byteOff {
			return
		}
		span := int(end - start)
		if bestSpan < 0 || span < bestSpan {
			best = Hit{Reference: ref, StartByte: start, EndByte: end, IsDef: isDef}
			bestSpan = span
		}
	}
	for _, t := range st.Rows(store.RelationAtom) {
		if len(t) < 6 || !sameStoreFile(t[0], fileRel) {
			continue
		}
		fp := strings.TrimPrefix(filepath.ToSlash(t[0]), "./")
		consider(AtomRef("./"+fp, t[1]), store.Atoi(t[2]), store.Atoi(t[3]), true)
	}
	for _, t := range st.Rows(store.RelationBinds) {
		if len(t) < 5 || !sameStoreFile(t[0], fileRel) {
			continue
		}
		target := t[4]
		if target == "" {
			target = t[3]
		}
		consider(target, store.Atoi(t[1]), store.Atoi(t[2]), false)
	}
	if bestSpan < 0 {
		return Hit{}, false
	}
	return best, true
}

func sameStoreFile(a, b string) bool {
	return strings.TrimPrefix(filepath.ToSlash(a), "./") == strings.TrimPrefix(filepath.ToSlash(b), "./")
}

// IdentifierAt returns the identifier token around byteOff in text.
func IdentifierAt(text string, byteOff int) (tok string, start, end int) {
	if byteOff < 0 {
		byteOff = 0
	}
	if byteOff > len(text) {
		byteOff = len(text)
	}
	isID := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
	}
	// Expand left
	start = byteOff
	for start > 0 {
		r, size := decodeLastRune(text[:start])
		if !isID(r) {
			break
		}
		start -= size
	}
	end = byteOff
	for end < len(text) {
		r, size := decodeRune(text[end:])
		if !isID(r) {
			break
		}
		end += size
	}
	if start >= end {
		return "", start, end
	}
	return text[start:end], start, end
}

func decodeRune(s string) (rune, int) {
	if s == "" {
		return 0, 0
	}
	r := rune(s[0])
	if r < 0x80 {
		return r, 1
	}
	// multi-byte: use range
	for _, rr := range s {
		return rr, len(string(rr))
	}
	return 0, 0
}

func decodeLastRune(s string) (rune, int) {
	if s == "" {
		return 0, 0
	}
	// walk back for UTF-8
	i := len(s) - 1
	for i > 0 && s[i]&0xc0 == 0x80 {
		i--
	}
	r, size := decodeRune(s[i:])
	return r, size
}
