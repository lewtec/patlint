package ingest

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/store"
)

// PruneNamedUnusedFromExtract drops unused named as-import sites (lisp marks).
// Barrels (star / blank / dot) stay. Empty ImportDefs → nil.
func PruneNamedUnusedFromExtract(fileRel string, content []byte, fe *project.FileExtract, opts PruneImportOpts) []project.Edit {
	if fe == nil || len(content) == 0 {
		return nil
	}
	sites := importSitesFromExtract(fe, content)
	if len(sites) == 0 {
		return nil
	}
	return PruneUnusedImportSites(fileRel, content, opts, sites, nil)
}

func importSitesFromExtract(fe *project.FileExtract, content []byte) []ImportSite {
	var sites []ImportSite
	for _, imp := range fe.Imports {
		start, end := importSiteSpan(imp, content)
		if end <= start {
			continue
		}
		locals := namedImportLocals(imp, content)
		if len(locals) == 0 {
			local := imp.LocalName
			if local == "" {
				local = LastPathComponent(imp.SourcePath)
			}
			if local != "" {
				locals = []string{local}
			}
		}
		site := ImportSite{
			Key:  imp.SourcePath,
			Span: ingestutil.Span{StartByte: start, EndByte: end},
		}
		keep := imp.MemberName == "*" || len(locals) == 0
		for _, local := range locals {
			if local == "*" || local == "." || local == "_" {
				keep = true
				break
			}
		}
		if keep {
			site.KeepAlways = true
		} else {
			site.Locals = locals
		}
		sites = append(sites, site)
	}
	return sites
}

// importSiteSpan is the marked as-import range, expanded to the line so a
// path-token mark still deletes the spec (Go import "x", JS from 'x').
func importSiteSpan(imp project.ImportDef, content []byte) (start, end uint32) {
	start, end = imp.StartByte, imp.EndByte
	if end <= start {
		start, end = ImportPathSpan(imp)
	}
	if end <= start || int(end) > len(content) {
		return 0, 0
	}
	for start > 0 && content[start-1] != '\n' {
		start--
	}
	// A mark that already ends on its newline must not walk into the next spec
	// (`"context"\n` + `"fmt"\n` in a Go import block).
	if end > start && content[end-1] == '\n' {
		return start, end
	}
	for end < uint32(len(content)) && content[end] != '\n' {
		end++
	}
	if end < uint32(len(content)) && content[end] == '\n' {
		end++
	}
	return start, end
}

// EnsureImportAfterLastMarked clones the last as-import, substituting need paths.
// No marked imports → nil (caller uses EnsureImportFirst).
func EnsureImportAfterLastMarked(fileRel string, content []byte, fe *project.FileExtract, needs []ImportNeed) []project.Edit {
	if fe == nil || len(needs) == 0 || len(content) == 0 {
		return nil
	}
	last, ok := lastMarkedImport(fe)
	if !ok {
		return nil
	}
	have := map[string]bool{}
	for _, imp := range fe.Imports {
		if imp.SourcePath != "" {
			have[imp.SourcePath] = true
		}
	}
	var extras []string
	for _, n := range needs {
		p := strings.TrimSpace(n.ImportPath)
		if p == "" || have[p] {
			continue
		}
		have[p] = true
		cloned := cloneImportWithPath(content, last, p)
		if cloned == "" {
			continue
		}
		extras = append(extras, cloned)
	}
	if len(extras) == 0 {
		return nil
	}
	nl := "\n"
	if last.EndByte > 0 && content[last.EndByte-1] == '\n' {
		nl = ""
	}
	insert := nl + strings.Join(extras, "\n")
	if insert[len(insert)-1] != '\n' && last.EndByte < uint32(len(content)) && content[last.EndByte] != '\n' {
		insert += "\n"
	}
	return []project.Edit{{
		File:    strings.TrimPrefix(fileRel, "./"),
		Span:    ingestutil.Span{StartByte: last.EndByte, EndByte: last.EndByte},
		NewText: insert,
	}}
}

// EnsureImportFirst inserts emit-seq as-import lines when the file has no marked imports.
func EnsureImportFirst(fileRel string, content []byte, fe *project.FileExtract, info project.ImportLineClaim, policy PackQueries, needs []ImportNeed) []project.Edit {
	if !importLineOkay(info) || len(needs) == 0 || len(content) == 0 {
		return nil
	}
	have := map[string]bool{}
	if fe != nil {
		for _, imp := range fe.Imports {
			if imp.SourcePath != "" {
				have[imp.SourcePath] = true
			}
		}
	}
	var lines []string
	for _, n := range needs {
		p := strings.TrimSpace(n.ImportPath)
		if p == "" || have[p] {
			continue
		}
		have[p] = true
		line := emitImportTokens(info, p, "", "")
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil
	}
	text := strings.Join(lines, "")
	if e, ok := rewriteLayoutUnit(store.RelationImport, fileRel, content, fe, nil, policy, text); ok {
		return []project.Edit{e}
	}
	return nil
}

func afterLine(content []byte, pos uint32) uint32 {
	i := int(pos)
	for i < len(content) && content[i] != '\n' {
		i++
	}
	if i < len(content) {
		return uint32(i + 1)
	}
	return uint32(len(content))
}

func lineStart(content []byte, pos uint32) uint32 {
	i := int(pos)
	if i > len(content) {
		i = len(content)
	}
	for i > 0 && content[i-1] != '\n' {
		i--
	}
	return uint32(i)
}

func firstMarkedImport(fe *project.FileExtract) (project.ImportDef, bool) {
	var first project.ImportDef
	ok := false
	for _, imp := range fe.Imports {
		if imp.EndByte <= imp.StartByte {
			continue
		}
		if !ok || imp.StartByte < first.StartByte {
			first = imp
			ok = true
		}
	}
	return first, ok
}

func lastMarkedImport(fe *project.FileExtract) (project.ImportDef, bool) {
	var last project.ImportDef
	ok := false
	for _, imp := range fe.Imports {
		if imp.EndByte <= imp.StartByte {
			continue
		}
		if !ok || imp.EndByte > last.EndByte {
			last = imp
			ok = true
		}
	}
	return last, ok
}

func cloneImportWithPath(content []byte, last project.ImportDef, newPath string) string {
	if int(last.EndByte) > len(content) || last.EndByte <= last.StartByte {
		return ""
	}
	spec := append([]byte(nil), content[last.StartByte:last.EndByte]...)
	ps, pe := last.PathStartByte, last.PathEndByte
	if pe <= ps || ps < last.StartByte || pe > last.EndByte {
		return ""
	}
	oldTok := content[ps:pe]
	newTok := retokenizeImportPath(oldTok, newPath)
	rel := int(ps - last.StartByte)
	out := append([]byte(nil), spec[:rel]...)
	out = append(out, newTok...)
	out = append(out, spec[rel+len(oldTok):]...)
	return string(out)
}

func retokenizeImportPath(oldTok []byte, newPath string) []byte {
	s := string(oldTok)
	if len(s) >= 2 {
		q := s[0]
		if (q == '"' || q == '\'' || q == '`') && s[len(s)-1] == q {
			return []byte(string(q) + newPath + string(q))
		}
	}
	return []byte(newPath)
}
