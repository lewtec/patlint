package ecma

import (
	"github.com/lewtec/patlint/pkg/project"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
)

// ComponentNameFromPath derives a PascalCase component name from a file path
// (e.g. search-check.svelte → SearchCheck).
func ComponentNameFromPath(relPath string) string {
	base := path.Base(relPath)
	base = strings.TrimSuffix(base, path.Ext(base))
	if base == "" || base == "." {
		return ""
	}
	parts := strings.FieldsFunc(base, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		r, size := utf8.DecodeRuneInString(p)
		if r == utf8.RuneError {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
		if len(p) > size {
			b.WriteString(p[size:])
		}
	}
	return b.String()
}

// EnsureDefaultExportFromPath sets fe.DefaultExport from the component file name
// when script did not declare one.
func EnsureDefaultExportFromPath(fe *project.FileExtract, relPath string) {
	if fe == nil || fe.DefaultExport != "" {
		return
	}
	if name := ComponentNameFromPath(relPath); name != "" {
		fe.DefaultExport = name
	}
}

// OffsetMergeExtract appends sub's atoms/imports/usages/reexports into fe,
// shifting byte ranges by start (host offset of the embedded region).
func OffsetMergeExtract(fe, sub *project.FileExtract, start uint32) {
	if fe == nil || sub == nil {
		return
	}
	for _, e := range sub.Atoms {
		e.StartByte += start
		e.EndByte += start
		fe.Atoms = append(fe.Atoms, e)
	}
	for _, im := range sub.Imports {
		im.StartByte += start
		im.EndByte += start
		if im.TargetStartByte != 0 || im.TargetEndByte != 0 {
			im.TargetStartByte += start
			im.TargetEndByte += start
		}
		fe.Imports = append(fe.Imports, im)
	}
	for _, u := range sub.Usages {
		u.StartByte += start
		u.EndByte += start
		for i := range u.Prefix {
			u.Prefix[i].StartByte += start
			u.Prefix[i].EndByte += start
		}
		fe.Usages = append(fe.Usages, u)
	}
	fe.Reexports = append(fe.Reexports, sub.Reexports...)
	if sub.DefaultExport != "" && fe.DefaultExport == "" {
		fe.DefaultExport = sub.DefaultExport
	}
}

// RawNodeSpan returns host [start,end) for a raw node, clamped to source.
func RawNodeSpan(raw *sitter.Node, source []byte) (start, end uint32, ok bool) {
	if raw == nil || raw.IsNull() {
		return 0, 0, false
	}
	start = raw.StartByte()
	end = raw.EndByte()
	if end > uint32(len(source)) {
		end = uint32(len(source))
	}
	if start >= end {
		return 0, 0, false
	}
	return start, end, true
}

// IsAllDigits reports whether s is non-empty and only ASCII digits.
func IsAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// MergeComponentTagUsage records a PascalCase (or dotted) component tag as a usage.
// reject, when non-nil, filters out host builtins (svelte:*, slot, …).
func MergeComponentTagUsage(fe *project.FileExtract, tag *sitter.Node, source []byte, reject func(name string) bool) {
	if fe == nil || tag == nil {
		return
	}
	name := ingestutil.NodeText(tag, source)
	if name == "" {
		return
	}
	r, _ := utf8.DecodeRuneInString(name)
	if r == utf8.RuneError || !unicode.IsUpper(r) {
		return
	}
	if reject != nil && reject(name) {
		return
	}
	fe.Usages = append(fe.Usages, project.UsageDef{
		Name:      name,
		StartByte: tag.StartByte(),
		EndByte:   tag.EndByte(),
	})
}
