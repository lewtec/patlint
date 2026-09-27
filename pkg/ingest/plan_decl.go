package ingest

import (
	"fmt"
	"github.com/lewtec/patlint/pkg/projectfs"
	"path"
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// extractDeclarationFromResult takes the as-decl hole that owns only this atom,
// or the innermost as-scope. A grouped spec (wrapper has another leaf)
// is pasted as first token of the as-decl span plus the spec. Host apply only.
func extractDeclarationFromResult(dir string, result *project.Result, entity project.Atom) (DeclExtract, error) {
	ref := ParseReference(entity.Reference)
	fileRelative := strings.TrimPrefix(ref.Path, "./")
	src, err := (projectfs.OS{}).ReadFile(path.Join(dir, fileRelative))
	if err != nil {
		return DeclExtract{}, err
	}
	start, end := entity.StartByte, entity.EndByte
	sc := coveringScope(result, fileRelative, start, end)
	sc = expandDeclarationHole(result, fileRelative, entity, sc)
	if sc != nil {
		start, end = sc.StartByte, sc.EndByte
	}
	if int(end) > len(src) {
		end = uint32(len(src))
	}
	if start > end {
		return DeclExtract{}, fmt.Errorf("declaration span inverted in %s", fileRelative)
	}
	declText := string(src[start:end])
	if kw := groupedDeclarationKeyword(src, result, fileRelative, sc); kw != "" {
		declText = applyDeclarationWrap(kw, declText, lineIndent(src, start))
		for end < uint32(len(src)) {
			c := src[end]
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				break
			}
			end++
		}
	} else {
		spanEnd := end
		for end < uint32(len(src)) && (src[end] == '\n' || src[end] == '\r') {
			end++
			if end-spanEnd >= 2 {
				break
			}
		}
		declText = string(src[start:end])
	}
	return DeclExtract{
		DeclText:    declText,
		RemoveStart: start,
		RemoveEnd:   end,
	}, nil
}

func expandDeclarationHole(result *project.Result, fileRelative string, entity project.Atom, sc *project.ScopeDef) *project.ScopeDef {
	if sc == nil || result == nil {
		return sc
	}
	for {
		p := scopeAt(result, fileRelative, sc.Parent)
		if p == nil || holeHasForeignLeaf(result, fileRelative, *p, *sc, entity) {
			return sc
		}
		sc = p
	}
}

func groupedDeclarationKeyword(src []byte, result *project.Result, fileRelative string, sc *project.ScopeDef) string {
	if sc == nil || sc.HoleOnly {
		return ""
	}
	for p := sc; p != nil && p.Parent >= 0; {
		p = scopeAt(result, fileRelative, p.Parent)
		if p == nil {
			return ""
		}
		if p.HoleOnly {
			return firstIdentifierToken(src, p.StartByte, p.EndByte)
		}
	}
	return ""
}

func firstIdentifierToken(src []byte, start, end uint32) string {
	i := int(start)
	e := int(end)
	if e > len(src) {
		e = len(src)
	}
	for i < e && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r') {
		i++
	}
	j := i
	for j < e {
		c := src[j]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' {
			j++
			continue
		}
		break
	}
	if j <= i {
		return ""
	}
	return string(src[i:j])
}

func holeHasForeignLeaf(result *project.Result, fileRelative string, parent, child project.ScopeDef, entity project.Atom) bool {
	if result == nil {
		return false
	}
	for _, a := range result.Atoms {
		r := ParseReference(a.Reference)
		if strings.TrimPrefix(r.Path, "./") != fileRelative {
			continue
		}
		if a.StartByte == entity.StartByte && a.EndByte == entity.EndByte && a.Reference == entity.Reference {
			continue
		}
		if strings.Contains(r.Name, ".") {
			continue
		}
		if a.StartByte >= child.StartByte && a.EndByte <= child.EndByte {
			continue
		}
		if a.StartByte >= parent.StartByte && a.EndByte <= parent.EndByte {
			return true
		}
	}
	return false
}

func scopeAt(result *project.Result, fileRelative string, idx int) *project.ScopeDef {
	if result == nil || idx < 0 {
		return nil
	}
	for _, f := range result.Files {
		if strings.TrimPrefix(f.Path, "./") != fileRelative {
			continue
		}
		if idx >= len(f.Scopes) {
			return nil
		}
		return &f.Scopes[idx]
	}
	return nil
}

func applyDeclarationWrap(kw, specifier, indent string) string {
	body := dedentPrefix(specifier, indent)
	if kw == "" {
		return body
	}
	return ingestutil.EmitSeq([]string{kw, body}, nil)
}

func dedentPrefix(s, prefix string) string {
	if prefix == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimPrefix(ln, prefix)
	}
	return strings.Join(lines, "\n")
}

func lineIndent(src []byte, start uint32) string {
	if int(start) > len(src) {
		return ""
	}
	i := int(start)
	for i > 0 && src[i-1] != '\n' && src[i-1] != '\r' {
		i--
	}
	j := i
	for j < int(start) && (src[j] == ' ' || src[j] == '\t') {
		j++
	}
	if j != int(start) {
		return ""
	}
	return string(src[i:start])
}

func coveringScope(result *project.Result, fileRelative string, start, end uint32) *project.ScopeDef {
	if result == nil {
		return nil
	}
	var best *project.ScopeDef
	bestSpan := uint32(1 << 30)
	for _, f := range result.Files {
		if strings.TrimPrefix(f.Path, "./") != fileRelative {
			continue
		}
		for i := range f.Scopes {
			s := &f.Scopes[i]
			if s.StartByte <= start && end <= s.EndByte {
				span := s.EndByte - s.StartByte
				if span < bestSpan && span > 0 {
					bestSpan = span
					best = s
				}
			}
		}
	}
	return best
}

func insertDeclarationGeneric(destinationRelative string, destinationContent []byte, decl DeclExtract, policy PackQueries, result *project.Result) project.Edit {
	if len(destinationContent) == 0 {
		pre := emptyDestinationPackagePreamble(policy, destinationRelative)
		return project.Edit{
			File:    destinationRelative,
			Span:    ingestutil.Span{StartByte: 0, EndByte: 0},
			NewText: ingestutil.AppendDeclText(pre, strings.TrimRight(decl.DeclText, "\n")),
		}
	}
	text := ""
	if destinationContent[len(destinationContent)-1] != '\n' {
		text = "\n"
	}
	text += "\n" + strings.TrimRight(decl.DeclText, "\n") + "\n"
	fileExtract := fileExtractForImports(result, destinationRelative, nil)
	e, ok := rewriteLayoutUnit(layoutBody, destinationRelative, destinationContent, fileExtract, result, policy, text)
	if !ok {
		return project.Edit{}
	}
	return e
}

// emptyDestinationPackagePreamble emits host (as-package (seq …)) for an empty dest.
func emptyDestinationPackagePreamble(policy PackQueries, destinationRelative string) string {
	if policy == nil {
		return ""
	}
	lang, ok, err := AttributeHost(policy, destinationRelative)
	if err != nil || !ok {
		return ""
	}
	claim := packageInfo(policy, lang)
	if len(claim.Tokens) == 0 {
		return ""
	}
	fileRelative := strings.TrimPrefix(destinationRelative, "./")
	pkg := LastPathComponent(path.Dir(fileRelative))
	if pkg == "" || pkg == "." {
		base := path.Base(fileRelative)
		for _, ext := range policy.PathExtsForLanguage(lang) {
			if ext != "" && !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			if ext != "" && strings.HasSuffix(base, ext) {
				base = strings.TrimSuffix(base, ext)
				break
			}
		}
		pkg = LastPathComponent(base)
	}
	if pkg == "" || pkg == "." {
		return ""
	}
	s := ingestutil.EmitSeq(claim.Tokens, map[string]string{"pkg": pkg})
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}
