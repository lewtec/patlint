package ingestgo

import (
	"fmt"
	"strconv"
	"strings"
)

type goImportSpec struct {
	local      string
	path       string
	lineStart  int
	lineEnd    int
	blockStart int
	blockEnd   int
}

func parseGoImportSpecString(s string) (alias, path string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if i := strings.IndexByte(s, ' '); i > 0 {
		a, p := s[:i], strings.TrimSpace(s[i+1:])
		if a != "" && p != "" && !strings.Contains(a, "/") && !strings.Contains(a, ".") {
			return a, p
		}
	}
	return "", s
}

func parseGoImportSpecs(source []byte) []goImportSpec {
	code := goCodeMask(source)
	text := string(source)
	var specs []goImportSpec
	lines := strings.Split(text, "\n")
	inBlock := false
	blockStart := -1
	offset := 0
	for _, line := range lines {
		lineLen := len(line)
		lineEnd := offset + lineLen
		if lineEnd < len(text) {
			lineEnd++
		}
		trimStart := offset
		for trimStart < lineEnd && (trimStart < len(source) && (source[trimStart] == ' ' || source[trimStart] == '\t')) {
			trimStart++
		}
		inCode := trimStart < len(code) && code[trimStart]
		trim := strings.TrimSpace(line)
		if !inBlock {
			if !inCode {
				offset = lineEnd
				continue
			}
			if trim == "import (" {
				inBlock = true
				blockStart = offset
				offset = lineEnd
				continue
			}
			if strings.HasPrefix(trim, "import ") {
				if spec, ok := parseGoImportLine(strings.TrimSpace(strings.TrimPrefix(trim, "import "))); ok {
					spec.lineStart = offset
					spec.lineEnd = lineEnd
					spec.blockStart = -1
					spec.blockEnd = -1
					specs = append(specs, spec)
				}
			}
			offset = lineEnd
			continue
		}
		if trim == ")" {
			for i := range specs {
				if specs[i].blockStart == blockStart && specs[i].blockEnd == 0 {
					specs[i].blockEnd = lineEnd
				}
			}
			inBlock = false
			blockStart = -1
			offset = lineEnd
			continue
		}
		if trim == "" || strings.HasPrefix(trim, "//") {
			offset = lineEnd
			continue
		}
		if spec, ok := parseGoImportLine(trim); ok {
			spec.lineStart = offset
			spec.lineEnd = lineEnd
			spec.blockStart = blockStart
			spec.blockEnd = 0
			specs = append(specs, spec)
		}
		offset = lineEnd
	}
	return specs
}

func goCodeMask(src []byte) []bool {
	mask := make([]bool, len(src))
	for i := 0; i < len(src); {
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			if i+1 < len(src) {
				i += 2
			}
			continue
		}
		if src[i] == '"' {
			i++
			for i < len(src) && src[i] != '"' {
				if src[i] == '\\' && i+1 < len(src) {
					i += 2
					continue
				}
				i++
			}
			if i < len(src) {
				i++
			}
			continue
		}
		if src[i] == '`' {
			i++
			for i < len(src) && src[i] != '`' {
				i++
			}
			if i < len(src) {
				i++
			}
			continue
		}
		if src[i] == '\'' {
			i++
			for i < len(src) && src[i] != '\'' {
				if src[i] == '\\' && i+1 < len(src) {
					i += 2
					continue
				}
				i++
			}
			if i < len(src) {
				i++
			}
			continue
		}
		mask[i] = true
		i++
	}
	return mask
}

func parseGoImportLine(s string) (goImportSpec, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return goImportSpec{}, false
	}
	local := ""
	pathPart := s
	if s[0] == '"' || s[0] == '`' {
		pathPart = s
	} else {
		parts := strings.Fields(s)
		if len(parts) < 2 {
			return goImportSpec{}, false
		}
		local = parts[0]
		pathPart = parts[1]
	}
	pathPart = strings.TrimSpace(pathPart)
	if len(pathPart) < 2 {
		return goImportSpec{}, false
	}
	quote := pathPart[0]
	if quote != '"' && quote != '`' {
		return goImportSpec{}, false
	}
	end := strings.IndexByte(pathPart[1:], quote)
	if end < 0 {
		return goImportSpec{}, false
	}
	p := pathPart[1 : 1+end]
	if local == "" {
		local = goAssumedImportName(p)
	}
	return goImportSpec{local: local, path: p}, true
}

func goAssumedImportName(importPath string) string {
	importPath = strings.TrimSuffix(importPath, "/")
	base := importPath
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if len(base) > 1 && base[0] == 'v' {
		if _, err := strconv.Atoi(base[1:]); err == nil {
			dir := importPath
			if j := strings.LastIndex(dir, "/"); j >= 0 {
				dir = dir[:j]
				if k := strings.LastIndex(dir, "/"); k >= 0 {
					base = dir[k+1:]
				} else if dir != "" {
					base = dir
				}
			}
		}
	}
	base = strings.TrimPrefix(base, "go-")
	if i := strings.IndexFunc(base, notGoIdentRune); i >= 0 {
		base = base[:i]
	}
	return base
}

func notGoIdentRune(r rune) bool {
	return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_')
}

// EnsureImportsInContent adds missing import paths to Go source text.
func EnsureImportsInContent(content string, paths []string) string {
	if len(paths) == 0 {
		return content
	}
	existing := map[string]bool{}
	for _, spec := range parseGoImportSpecs([]byte(content)) {
		existing[spec.path] = true
	}
	var missing []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		_, pathOnly := parseGoImportSpecString(p)
		if pathOnly == "" || existing[pathOnly] {
			continue
		}
		existing[pathOnly] = true
		missing = append(missing, p)
	}
	if len(missing) == 0 {
		return content
	}
	block := formatGoImportBlock(missing)
	lines := strings.Split(content, "\n")
	insertAt := 0
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			insertAt = i + 1
			break
		}
	}
	for insertAt < len(lines) && strings.TrimSpace(lines[insertAt]) == "" {
		insertAt++
	}
	if insertAt < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[insertAt]), "import ") {
		return mergeIntoExistingGoImports(content, missing)
	}
	out := make([]string, 0, len(lines)+len(missing)+3)
	out = append(out, lines[:insertAt]...)
	if insertAt > 0 && insertAt <= len(lines) {
		out = append(out, "")
	}
	out = append(out, strings.Split(strings.TrimSuffix(block, "\n"), "\n")...)
	out = append(out, "")
	out = append(out, lines[insertAt:]...)
	return strings.Join(out, "\n")
}

func formatGoImportLine(spec string) string {
	alias, path := parseGoImportSpecString(spec)
	if alias != "" {
		return fmt.Sprintf("%s %q", alias, path)
	}
	return fmt.Sprintf("%q", path)
}

func formatGoImportBlock(paths []string) string {
	if len(paths) == 1 {
		return "import " + formatGoImportLine(paths[0])
	}
	var b strings.Builder
	b.WriteString("import (\n")
	for _, p := range paths {
		b.WriteString("\t" + formatGoImportLine(p) + "\n")
	}
	b.WriteString(")")
	return b.String()
}

func mergeIntoExistingGoImports(content string, missing []string) string {
	specs := parseGoImportSpecs([]byte(content))
	have := map[string]bool{}
	for _, s := range specs {
		have[s.path] = true
	}
	var add []string
	for _, p := range missing {
		_, pathOnly := parseGoImportSpecString(p)
		if pathOnly == "" || have[pathOnly] {
			continue
		}
		have[pathOnly] = true
		add = append(add, p)
	}
	if len(add) == 0 {
		return content
	}
	text := content
	if idx := strings.Index(text, "import ("); idx >= 0 {
		end := strings.Index(text[idx:], "\n)")
		if end >= 0 {
			insertPos := idx + end + 1
			var b strings.Builder
			for _, p := range add {
				b.WriteString("\t" + formatGoImportLine(p) + "\n")
			}
			return text[:insertPos] + b.String() + text[insertPos:]
		}
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "import ") {
			var extra []string
			for _, p := range add {
				extra = append(extra, "import "+formatGoImportLine(p))
			}
			out := append([]string{}, lines[:i+1]...)
			out = append(out, extra...)
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n")
		}
	}
	return EnsureImportsInContent(content, add)
}
