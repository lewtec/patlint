package ingest

import (
	"strings"

	"github.com/lewtec/patlint/pkg/project"
)

// Leftover dest-import POLICY in Go.
//
// These decide language shape that is not a pack/store fact.
// qualify.go apply should call these, not re-decide them.
// Do not add another language rule here. Next cut moves each to .rft.
//
//   destinationImportLine           — "as" alias is not an as-import field
//   destinationImportGroupInsert    — Go import ( ) group
//   wrapCopiedImports               — synthesizes "import (\n…\n)"
//   destinationImportText           — `"path"` / `alias "path"` not from seq
//   destinationImportSpecifier      — go.mod module path
//   destinationRelativeSpecifier    — quoted ./rel vs module vs HasQual path
//   destinationFileImportSpecifier  — HasLeaf/HasQual pick among those

func destinationImportLine(importLine project.ImportLineClaim, specifier, leaf, alias string) string {
	if specifier == "" || leaf == "" || !importLineOkay(importLine) {
		return ""
	}
	if alias != "" && alias != leaf {
		leaf = leaf + " as " + alias
	}
	return emitImportLine(importLine, specifier, leaf, "")
}

func destinationImportGroupInsert(destination []byte, destinationExtract *project.FileExtract, extras []string, importLine project.ImportLineClaim) (uint32, string) {
	if !importLine.QuotedPath || importLine.HasQual || importLine.HasLeaf || len(destination) == 0 || destinationExtract == nil {
		return 0, ""
	}
	imp, ok := lastMarkedImport(destinationExtract)
	if !ok {
		return 0, ""
	}
	at := afterLine(destination, imp.EndByte)
	i := int(at)
	for i < len(destination) && (destination[i] == ' ' || destination[i] == '\t' || destination[i] == '\n') {
		i++
	}
	if i >= len(destination) || destination[i] != ')' {
		return 0, ""
	}
	return at, formatGroupSpecs(extras)
}

func formatGroupSpecs(extras []string) string {
	var b strings.Builder
	for _, e := range extras {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, "\t") {
			b.WriteByte('\t')
		}
		b.WriteString(e)
		b.WriteByte('\n')
	}
	return b.String()
}

func wrapCopiedImports(extras []string, importLine project.ImportLineClaim) string {
	if importLine.HasQual || importLine.HasLeaf || !importLine.QuotedPath {
		return strings.Join(extras, "\n") + "\n"
	}
	if len(extras) == 0 {
		return ""
	}
	if len(extras) == 1 {
		specifier := strings.Trim(extras[0], `"'`)
		if !strings.Contains(extras[0], " ") {
			return strings.TrimLeft(emitImportTokens(importLine, specifier, "", ""), "\n")
		}
	}
	var b strings.Builder
	b.WriteString("import (\n")
	for _, s := range extras {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		b.WriteByte('\t')
		b.WriteString(s)
		b.WriteByte('\n')
	}
	b.WriteString(")\n")
	return b.String()
}

func destinationImportSpecifier(dir string, source, destination Reference, result *project.Result) string {
	oldKey, newKey := importRewriteKeys(source, destination)
	if result != nil && oldKey != "" && newKey != "" {
		for _, a := range result.Aliases {
			np := RewriteImportPathDir(a.ImportPath, oldKey, newKey)
			if np != "" && np != a.ImportPath {
				return np
			}
		}
	}
	dstDir := importFileDir(strings.TrimPrefix(destination.Path, "./"))
	if mod := readGoModulePath(dir); mod != "" {
		if dstDir != "" {
			return strings.Trim(mod, "/") + "/" + dstDir
		}
		return strings.Trim(mod, "/")
	}
	return ""
}

func destinationRelativePath(importerRel, destinationRelative string) string {
	rel := destinationFileRelative(importerRel, destinationRelative)
	if rel != "" && !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel
}

func destinationRelativeSpecifier(importerRel, destinationRelative string, info project.ImportLineClaim, dir string, policy PackQueries) string {
	destinationRelative = strings.TrimPrefix(filepathToSlashImport(destinationRelative), "./")
	if destinationRelative == "" {
		return ""
	}
	if info.HasQual {
		return strings.TrimPrefix(destinationRelativePath(importerRel, destinationRelative), "./")
	}
	if info.QuotedPath {
		rel := destinationRelativePath(importerRel, destinationRelative)
		rel = trimSpecifierExtension(rel)
		if rel != "" && !strings.HasPrefix(rel, ".") {
			rel = "./" + rel
		}
		return rel
	}
	return destinationModuleSpecifier(dir, importerRel, destinationRelative, policy)
}

func destinationFileImportSpecifier(dir string, source, destination Reference, result *project.Result, policy PackQueries, importerRel string, lineInfo project.ImportLineClaim) string {
	destinationRelative := strings.TrimPrefix(destination.Path, "./")
	if lineInfo.HasLeaf || lineInfo.HasQual {
		return destinationRelativeSpecifier(importerRel, destinationRelative, lineInfo, dir, policy)
	}
	if specifier := destinationImportSpecifier(dir, source, destination, result); specifier != "" {
		return specifier
	}
	return destinationRelativeSpecifier(importerRel, destinationRelative, lineInfo, dir, policy)
}

func destinationImportText(importLine project.ImportLineClaim, sourceRelative, destinationRelative, dir string, policy PackQueries, im project.ImportDef, source []byte) string {
	info := importLine
	specifier := im.SourcePath
	if specifier == "" || !importLineOkay(info) {
		return ""
	}
	if np := importSpecifier(specifier).rebase(sourceRelative, destinationRelative); np != "" {
		specifier = np
	} else if looksLikeFileSpecifier(specifier) {
		if resolved := importSpecifier(specifier).resolveFrom(sourceRelative); resolved != "" {
			if np := destinationRelativeSpecifier(destinationRelative, resolved, info, dir, policy); np != "" {
				specifier = np
			}
		}
	}
	local := importLocalName(im)
	pathOnly := im.MemberName == "" && (im.LocalName == "" || im.LocalName == LastPathComponent(im.SourcePath) || im.LocalName == fileStem(im.SourcePath))
	if info.QuotedPath && !info.HasQual && !info.HasLeaf {
		if !pathOnly && im.LocalName != "" && im.LocalName != LastPathComponent(im.SourcePath) {
			return im.LocalName + " \"" + specifier + "\""
		}
		return "\"" + specifier + "\""
	}
	if info.HasQual {
		return strings.TrimRight(emitImportTokens(info, specifier, "", local), "\n")
	}
	if pathOnly {
		if len(source) == 0 || int(im.StartByte) >= len(source) {
			return ""
		}
		ls := lineStart(source, im.StartByte)
		le := afterLine(source, ls)
		if le > ls {
			return strings.TrimSpace(string(source[ls:le]))
		}
		return ""
	}
	leaf := im.MemberName
	if leaf == "" {
		leaf = local
	}
	return strings.TrimRight(emitImportTokens(info, specifier, leaf, fileStem(specifier)), "\n")
}
