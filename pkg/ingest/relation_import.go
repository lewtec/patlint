package ingest

import (
	"bytes"
	"strings"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/store"
)

// Import relation projection. Mechanism. No language names, no pack knobs.
//
// Truth is store.RelationImport. Result.Aliases is fallback when the store
// has no import rows for the file.
//
// Identity (importLocalName, namedImportLocals, importDeduplicateKey,
// specifierNamesMovedSource, restyleSpecifierStem) lives here — not on apply.

func importDefsFromRelationImport(storeRows *store.Store, fileRelative string) []project.ImportDef {
	if storeRows == nil {
		return nil
	}
	fileRelative = strings.TrimPrefix(fileRelative, "./")
	var out []project.ImportDef
	for _, row := range storeRows.Rows(store.RelationImport) {
		if len(row) < 6 || !sameStoreFile(row[0], fileRelative) {
			continue
		}
		def := project.ImportDef{
			LocalName:  row[1],
			SourcePath: row[2],
			MemberName: row[3],
			StartByte:  store.Atoi(row[4]),
			EndByte:    store.Atoi(row[5]),
		}
		if len(row) > 10 {
			def.PathStartByte = store.Atoi(row[9])
			def.PathEndByte = store.Atoi(row[10])
		}
		if len(row) > 8 {
			def.HasAliasBinding = row[8] == "1"
		}
		out = append(out, def)
	}
	return out
}

func fileExtractForImports(result *project.Result, fileRelative string, storeRows *store.Store) *project.FileExtract {
	extract := &project.FileExtract{Path: fileRelative}
	fileRelative = strings.TrimPrefix(fileRelative, "./")
	if result != nil {
		for _, f := range result.Files {
			if strings.TrimPrefix(f.Path, "./") != fileRelative {
				continue
			}
			extract.Language = f.Language
			extract.Package = f.Package
			extract.PackageEnd = f.PackageEnd
			extract.Scopes = f.Scopes
		}
	}
	if name, end, ok := packageNameFromRelationPackage(storeRows, fileRelative); ok {
		extract.Package = name
		extract.PackageEnd = end
	}
	extract.Imports = importDefsFromRelationImport(storeRows, fileRelative)
	if len(extract.Imports) == 0 && result != nil {
		for _, a := range result.Aliases {
			r := ParseReference(a.Reference)
			if strings.TrimPrefix(r.Path, "./") != fileRelative {
				continue
			}
			if a.ImportPath == "" || a.ImportPathEnd <= a.ImportPathStart {
				continue
			}
			extract.Imports = append(extract.Imports, project.ImportDef{
				SourcePath:    a.ImportPath,
				PathStartByte: a.ImportPathStart,
				PathEndByte:   a.ImportPathEnd,
				StartByte:     a.StartByte,
				EndByte:       a.EndByte,
			})
		}
	}
	return extract
}

func importStatementSpan(src []byte, pos uint32) (uint32, uint32) {
	ls := lineStart(src, pos)
	for ls > 0 {
		prev := lineStart(src, ls-1)
		if prev >= ls || !importGroupOpens(src[prev:ls]) {
			break
		}
		ls = prev
	}
	le := afterLine(src, ls)
	if ls >= le || int(le) > len(src) {
		return ls, le
	}
	if importGroupOpens(src[ls:le]) {
		i := int(le)
		for i < len(src) && src[i-1] != ')' && src[i-1] != '}' {
			i++
		}
		return ls, uint32(i)
	}
	return ls, le
}

func importGroupOpens(line []byte) bool {
	return (bytes.Contains(line, []byte("(")) && !bytes.Contains(line, []byte(")"))) ||
		(bytes.Contains(line, []byte("{")) && !bytes.Contains(line, []byte("}")))
}

func eachImportStatement(src []byte, fileExtract *project.FileExtract, fn func(ls, le uint32, importDef project.ImportDef) bool) {
	if fileExtract == nil || len(src) == 0 || fn == nil {
		return
	}
	seen := map[[2]uint32]bool{}
	for _, importDef := range fileExtract.Imports {
		pos := importDef.StartByte
		if importDef.EndByte <= importDef.StartByte && importDef.PathEndByte > importDef.PathStartByte {
			pos = importDef.PathStartByte
		}
		if int(pos) >= len(src) {
			continue
		}
		ls, le := importStatementSpan(src, pos)
		if le <= ls {
			continue
		}
		key := [2]uint32{ls, le}
		if seen[key] {
			continue
		}
		seen[key] = true
		if !fn(ls, le, importDef) {
			return
		}
	}
}

func importStatementSpecifier(importDef project.ImportDef, _ []byte) string {
	return importDef.SourcePath
}

// importLocalName is the name this import relation binds in the importer.
func importLocalName(importDef project.ImportDef) string {
	if importDef.LocalName != "" && importDef.LocalName != LastPathComponent(importDef.SourcePath) {
		return importDef.LocalName
	}
	if importDef.MemberName != "" {
		return importDef.MemberName
	}
	return fileStem(importDef.SourcePath)
}

func importDeduplicateKey(importDef project.ImportDef) string {
	specifier := strings.Trim(importDef.SourcePath, `"'`)
	if importDef.MemberName != "" {
		return specifier + "\t" + importDef.MemberName
	}
	if importDef.LocalName != "" && importDef.LocalName != LastPathComponent(specifier) {
		return specifier + "\t" + importDef.LocalName
	}
	return specifier
}

func namedImportLocals(importDef project.ImportDef, src []byte) []string {
	if importDef.MemberName != "" {
		return []string{importDef.MemberName}
	}
	if importDef.LocalName != "" && !strings.ContainsAny(importDef.LocalName, " \t{") {
		return []string{importDef.LocalName}
	}
	if importDef.EndByte > importDef.StartByte && int(importDef.EndByte) <= len(src) {
		if loc := braceImportLocals(string(src[importDef.StartByte:importDef.EndByte])); len(loc) > 0 {
			return loc
		}
	}
	return nil
}

// braceImportLocals reads `{name as alias, …}` members when the import relation
// rows did not capture them. The " as " token is leftover until name fields do.
func braceImportLocals(s string) []string {
	i := strings.Index(s, "{")
	j := strings.Index(s, "}")
	if i < 0 || j <= i {
		return nil
	}
	var out []string
	for p := range strings.SplitSeq(s[i+1:j], ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if k := strings.Index(p, " as "); k >= 0 {
			p = strings.TrimSpace(p[k+4:])
		}
		out = append(out, p)
	}
	return out
}

func specifierNamesMovedSource(specifier, importerRel, sourceRelative string, result *project.Result) bool {
	if importSpecifier(specifier).namesFile(sourceRelative) || importSpecifier(specifier).namesFileFrom(importerRel, sourceRelative, nil) {
		return true
	}
	if result == nil || specifier == "" {
		return false
	}
	if LastPathComponent(strings.ReplaceAll(specifier, ".", "/")) != fileStem(sourceRelative) {
		return false
	}
	want := strings.TrimPrefix(sourceRelative, "./")
	imp := strings.TrimPrefix(importerRel, "./")
	for _, a := range result.Aliases {
		if a.ImportPath != specifier {
			continue
		}
		if strings.TrimPrefix(ParseReference(a.Reference).Path, "./") != imp {
			continue
		}
		name := AtomName(ParseReference(a.Target).Name)
		if name == "" {
			continue
		}
		if strings.TrimPrefix(ParseReference(a.Target).Path, "./") == want {
			return true
		}
		for _, at := range result.Atoms {
			ar := ParseReference(at.Reference)
			if strings.TrimPrefix(ar.Path, "./") == want && AtomName(ar.Name) == name {
				return true
			}
		}
	}
	return false
}

func restyleSpecifierStem(specifier, sourceRelative, destinationRelative string) string {
	if specifier == "" {
		return ""
	}
	np := RewriteImportPathDir(specifier, fileStem(sourceRelative), fileStem(destinationRelative))
	if np == "" || np == specifier {
		return ""
	}
	return np
}
