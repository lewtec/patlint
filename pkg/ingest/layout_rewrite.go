package ingest

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/store"
)

const layoutBody = "body"

// rewriteLayoutUnit is the rewrite loop for a new unit in as-layout NAME.
// Last ingest site of that name: EMIT = (seq (slot) neu).
// Empty section: zero-width slot after the previous name, EMIT = neu.
func rewriteLayoutUnit(name, fileRel string, src []byte, fileExtract *project.FileExtract, result *project.Result, policy PackQueries, neu string) (project.Edit, bool) {
	if neu == "" || policy == nil {
		return project.Edit{}, false
	}
	lang, ok := layoutFileLanguage(policy, fileRel, fileExtract)
	if !ok {
		return project.Edit{}, false
	}
	names := policy.Layout(lang)
	if len(names) == 0 {
		names = []string{layoutBody}
	}
	if !layoutContains(names, name) {
		return project.Edit{}, false
	}
	fileRel = trimLayoutRelative(fileRel)
	if sp, ok := layoutLastUnit(name, names, src, fileExtract, result, fileRel); ok {
		if int(sp.EndByte) <= len(src) && sp.StartByte <= sp.EndByte {
			old := string(src[sp.StartByte:sp.EndByte])
			return project.Edit{
				File:    fileRel,
				Span:    sp,
				NewText: old + neu,
			}, true
		}
	}
	at := layoutEmptySlot(name, names, src, fileExtract, result, fileRel, policy)
	return project.Edit{
		File:    fileRel,
		Span:    ingestutil.Span{StartByte: at, EndByte: at},
		NewText: neu,
	}, true
}

func layoutFileLanguage(policy PackQueries, fileRelative string, fileExtract *project.FileExtract) (string, bool) {
	if policy != nil {
		if lang, ok, err := AttributeHost(policy, fileRelative); err == nil && ok {
			return lang, true
		}
	}
	if fileExtract != nil && fileExtract.Language != "" {
		return fileExtract.Language, true
	}
	return "", false
}

func layoutContains(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

func trimLayoutRelative(fileRelative string) string {
	for len(fileRelative) >= 2 && fileRelative[0] == '.' && fileRelative[1] == '/' {
		fileRelative = fileRelative[2:]
	}
	return fileRelative
}

func layoutLastUnit(name string, names []string, src []byte, fileExtract *project.FileExtract, result *project.Result, fileRelative string) (ingestutil.Span, bool) {
	switch name {
	case store.RelationPackage:
		if fileExtract != nil && fileExtract.PackageEnd > 0 {
			return coveringLine(src, fileExtract.PackageEnd-1), true
		}
	case store.RelationImport:
		if pos, ok := lastImportPosition(fileExtract, result, fileRelative); ok {
			return coveringLine(src, pos), true
		}
	case layoutBody:
		return layoutBodyResidual(names, src, fileExtract, result, fileRelative)
	}
	return ingestutil.Span{}, false
}

func layoutBodyResidual(names []string, src []byte, fileExtract *project.FileExtract, result *project.Result, fileRelative string) (ingestutil.Span, bool) {
	if len(src) == 0 {
		return ingestutil.Span{}, false
	}
	start := uint32(0)
	for _, n := range names {
		if n == layoutBody {
			break
		}
		if sp, ok := layoutLastUnit(n, names, src, fileExtract, result, fileRelative); ok {
			start = sp.EndByte
		}
	}
	if start >= uint32(len(src)) {
		return ingestutil.Span{}, false
	}
	return ingestutil.Span{StartByte: start, EndByte: uint32(len(src))}, true
}

func layoutEmptySlot(name string, names []string, src []byte, fileExtract *project.FileExtract, result *project.Result, fileRelative string, policy PackQueries) uint32 {
	prev := ""
	for _, n := range names {
		if n == name {
			break
		}
		prev = n
	}
	if prev == "" {
		return 0
	}
	if sp, ok := layoutLastUnit(prev, names, src, fileExtract, result, fileRelative); ok {
		at := sp.EndByte
		for int(at) < len(src) && src[at] == '\n' {
			at++
		}
		return at
	}
	if len(src) == 0 && prev == store.RelationPackage {
		if pre := emptyDestinationPackagePreamble(policy, fileRelative); pre != "" {
			return uint32(len(pre))
		}
	}
	return layoutEmptySlot(prev, names, src, fileExtract, result, fileRelative, policy)
}

func lastImportPosition(fileExtract *project.FileExtract, result *project.Result, fileRelative string) (uint32, bool) {
	if fileExtract == nil {
		return 0, false
	}
	scopes := fileScopes(result, fileRelative)
	if len(fileExtract.Scopes) > 0 {
		scopes = fileExtract.Scopes
	}
	var pos uint32
	ok := false
	for _, importDef := range fileExtract.Imports {
		p := importSitePosition(importDef)
		if p == 0 || inInnerScope(result, fileRelative, scopes, p) {
			continue
		}
		if !ok || p > pos {
			pos = p
			ok = true
		}
	}
	return pos, ok
}

func importSitePosition(importDef project.ImportDef) uint32 {
	if importDef.EndByte > importDef.StartByte {
		return importDef.EndByte - 1
	}
	if importDef.PathEndByte > importDef.PathStartByte {
		return importDef.PathEndByte - 1
	}
	return 0
}

func coveringLine(src []byte, position uint32) ingestutil.Span {
	if len(src) == 0 {
		return ingestutil.Span{}
	}
	if int(position) >= len(src) {
		position = uint32(len(src) - 1)
	}
	return ingestutil.Span{StartByte: lineStart(src, position), EndByte: afterLine(src, position)}
}

func fileScopes(result *project.Result, fileRelative string) []project.ScopeDef {
	if result == nil {
		return nil
	}
	fileRelative = strings.TrimPrefix(fileRelative, "./")
	for _, f := range result.Files {
		if strings.TrimPrefix(f.Path, "./") == fileRelative {
			return f.Scopes
		}
	}
	return nil
}

func inInnerScope(result *project.Result, fileRelative string, scopes []project.ScopeDef, position uint32) bool {
	fileRelative = strings.TrimPrefix(fileRelative, "./")
	for _, sc := range scopes {
		if sc.HoleOnly || position < sc.StartByte || position >= sc.EndByte {
			continue
		}
		if result == nil {
			return true
		}
		for _, a := range result.Atoms {
			if strings.TrimPrefix(ParseReference(a.Reference).Path, "./") != fileRelative {
				continue
			}
			if a.StartByte >= sc.StartByte && a.StartByte < position {
				return true
			}
		}
	}
	return false
}
