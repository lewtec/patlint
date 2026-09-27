package ingest

import (
	"context"
	"github.com/lewtec/patlint/pkg/projectfs"
	"path"
	"slices"
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/store"
)

// Dest-import apply (mechanism).
//
//	pack       packMove          — .rft as-import / as-layout / as-family
//	facts      import relation   — relation_import.go
//	           package relation  — relation_package.go
//	           use leftover      — relation_use.go
//	sites      importFile        — import_site.go Composite
//	policy     move_policy.go    — leftover dest-path / emit-shape (grep that file)
//
// Do not add an if-lang or string template here. Pack or move_policy.go.

// moveQualify applies dest-import / use-qualify for one hole leaving its file.
type moveQualify struct {
	dir                 string
	result              *project.Result
	store               *store.Store
	source              Reference
	destination         Reference
	sourceRelative      string
	destinationRelative string
	language            string
	pack                packMove
	entity              project.Atom
	hole                *DeclExtract
	policy              PackQueries
}

func newMoveQualify(dir string, result *project.Result, store *store.Store, source, destination Reference, entity project.Atom, hole *DeclExtract, policy PackQueries) moveQualify {
	sourceRelative := strings.TrimPrefix(source.Path, "./")
	destinationRelative := strings.TrimPrefix(destination.Path, "./")
	language := languageForRefPath(result, sourceRelative)
	return moveQualify{
		dir:                 dir,
		result:              result,
		store:               store,
		source:              source,
		destination:         destination,
		sourceRelative:      sourceRelative,
		destinationRelative: destinationRelative,
		language:            language,
		pack:                loadPackMove(policy, language),
		entity:              entity,
		hole:                hole,
		policy:              policy,
	}
}

func (m moveQualify) edits(ctx context.Context) ([]project.Edit, error) {
	if m.result == nil || m.hole == nil || m.sourceRelative == m.destinationRelative {
		return nil, nil
	}
	var edits []project.Edit
	// Use relation / binds: rewrite names that stay behind.
	if !m.pack.sameMoveModule(m.sourceRelative, m.destinationRelative) {
		uses, err := m.qualifyUses(ctx)
		if err != nil {
			return nil, err
		}
		edits = append(edits, uses...)
	}
	dstBytes := readFileCached(m.dir, m.destinationRelative)
	// Import relation + hole uses: copy deps, leftover names.
	edits = append(edits, m.holeDependencyImports(dstBytes)...)
	edits = append(edits, m.holeLeftover(dstBytes)...)
	// Import-relation name fields (pack HasLeaf).
	if m.pack.importLine.HasLeaf {
		split, err := m.splitFromImports(ctx)
		if err != nil {
			return nil, err
		}
		edits = append(edits, split...)
		retarget, err := m.retargetLeafFrom(ctx)
		if err != nil {
			return nil, err
		}
		edits = append(edits, retarget...)
	}
	return edits, nil
}

func readFileCached(dir, relative string) []byte {
	b, err := (projectfs.OS{}).ReadFile(path.Join(dir, relative))
	if err != nil {
		return nil
	}
	return b
}

func (m moveQualify) qualifyUses(ctx context.Context) ([]project.Edit, error) {
	srcDir := importFileDir(m.sourceRelative)
	destinationQualifier := destinationPackageName(m.result, m.destinationRelative, importFileDir(m.destinationRelative))
	leaf := AtomName(m.source.Name)
	if leaf == "" {
		return nil, nil
	}
	named := m.pack.importLine.HasLeaf
	if destinationQualifier == "" && !named {
		destinationQualifier = fileStem(m.destinationRelative)
	}
	var edits []project.Edit
	needImport := map[string]bool{}
	contents := map[string][]byte{}
	read := func(rel string) []byte {
		if b, ok := contents[rel]; ok {
			return b
		}
		b, err := (projectfs.OS{}).ReadFile(path.Join(m.dir, rel))
		if err != nil {
			return nil
		}
		contents[rel] = b
		return b
	}
	for _, u := range m.result.Uses {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !useTargetsMovedAtom(u, m.source, m.entity) {
			continue
		}
		uref := ParseReference(u.Reference)
		fileRel := strings.TrimPrefix(uref.Path, "./")
		if fileRel == m.destinationRelative || m.pack.sameMoveModule(fileRel, m.destinationRelative) {
			continue
		}
		if fileRel == m.sourceRelative && m.pack.importLine.HasQual {
			continue
		}
		if fileRel == m.sourceRelative && u.StartByte >= m.hole.RemoveStart && u.EndByte <= m.hole.RemoveEnd {
			continue
		}
		srcBytes := read(fileRel)
		if srcBytes == nil {
			continue
		}
		if destinationQualifier != "" {
			if e, ok := m.qualifySelectorEdit(fileRel, srcBytes, u.StartByte, u.EndByte, destinationQualifier); ok {
				edits = append(edits, e)
				if fileKeepsOldPackage(m.result, fileRel, m.sourceRelative, m.entity) {
					needImport[fileRel] = true
				}
				continue
			}
		}
		if !named && !m.pack.sameMoveModule(fileRel, m.sourceRelative) && importFileDir(fileRel) != srcDir {
			continue
		}
		if !named && destinationQualifier != "" {
			edits = append(edits, project.Edit{
				File:    fileRel,
				Span:    ingestutil.Span{StartByte: u.StartByte, EndByte: u.EndByte},
				NewText: destinationQualifier + "." + leaf,
			})
		}
		needImport[fileRel] = true
	}
	if m.pack.importLine.HasQual && destinationQualifier != "" {
		if leftover := m.leftoverIdentifierEdits(read(m.sourceRelative), leaf, destinationQualifier); len(leftover) > 0 {
			edits = append(edits, leftover...)
			needImport[m.sourceRelative] = true
		}
	}
	for fileRel := range needImport {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		srcBytes := read(fileRel)
		if srcBytes == nil {
			continue
		}
		fileExtract := fileExtractForImports(m.result, fileRel, m.store)
		if fileRel == m.sourceRelative && m.pack.rules.DestExport {
			fileExtract = m.dropHoleOnlyImports(fileExtract, srcBytes)
		}
		if loadImportFile(srcBytes, fileExtract).importsResolved(fileRel, m.destinationRelative, m.policy) {
			continue
		}
		lineInfo := m.pack.importLine
		if !importLineOkay(lineInfo) && fileExtract != nil {
			lineInfo = importInfo(m.policy, fileExtract.Language)
		}
		specifier := destinationFileImportSpecifier(m.dir, m.source, m.destination, m.result, m.policy, fileRel, lineInfo)
		imports := loadImportFile(srcBytes, fileExtract)
		if specifier != "" && !imports.hasOnlyMember(leaf) && !imports.hasOtherMembersFrom(m.sourceRelative, leaf, m.result) {
			edits = append(edits, m.insertDestinationImport(fileRel, srcBytes, fileExtract, specifier, lineInfo, leaf, destinationQualifier)...)
		}
	}
	return edits, nil
}

func (m moveQualify) insertDestinationImport(fileRel string, source []byte, fileExtract *project.FileExtract, specifier string, info project.ImportLineClaim, leaf, qualifier string) []project.Edit {
	if specifier == "" || !importLineOkay(info) || len(source) == 0 {
		return nil
	}
	if fileExtract != nil {
		for _, importDef := range fileExtract.Imports {
			if importDef.SourcePath == specifier || importSpecifier(importDef.SourcePath).namesFile(specifier) {
				return nil
			}
		}
	}
	line := emitImportLine(info, specifier, leaf, qualifier)
	if !info.HasLeaf && !info.HasQual && !strings.HasSuffix(line, "\n\n") {
		line += "\n"
	}
	if info.HasLeaf && !info.QuotedPath && !strings.HasSuffix(line, "\n\n") {
		line += "\n"
	}
	if e, ok := rewriteLayoutUnit(store.RelationImport, fileRel, source, fileExtract, nil, m.policy, line); ok {
		return []project.Edit{e}
	}
	return nil
}

func (m moveQualify) splitFromImports(ctx context.Context) ([]project.Edit, error) {
	leaf := AtomName(m.source.Name)
	if leaf == "" || m.result == nil {
		return nil, nil
	}
	files := map[string]bool{}
	for _, a := range m.result.Aliases {
		r := ParseReference(a.Reference)
		f := strings.TrimPrefix(r.Path, "./")
		if f == "" || f == m.sourceRelative || f == m.destinationRelative {
			continue
		}
		t := ParseReference(a.Target)
		if strings.TrimPrefix(t.Path, "./") == m.sourceRelative {
			files[f] = true
			continue
		}
		if specifierNamesMovedSource(a.ImportPath, f, m.sourceRelative, m.result) {
			files[f] = true
		}
	}
	var edits []project.Edit
	for f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b, err := (projectfs.OS{}).ReadFile(path.Join(m.dir, f))
		if err != nil {
			continue
		}
		imports := loadImportFile(b, fileExtractForImports(m.result, f, m.store))
		if !imports.hasOtherMembersFrom(m.sourceRelative, leaf, m.result) {
			continue
		}
		mod := imports.moduleFrom(m.sourceRelative, m.result)
		specifier := restyleSpecifierStem(mod, m.sourceRelative, m.destinationRelative)
		if specifier == "" {
			specifier = destinationFileImportSpecifier(m.dir, m.source, m.destination, m.result, m.policy, f, m.pack.importLine)
		}
		destLine := ""
		if specifier != "" && !imports.importsPath(m.destinationRelative) {
			alias := imports.aliasFor(m.sourceRelative, leaf)
			destLine = destinationImportLine(m.pack.importLine, specifier, leaf, alias)
			if destLine != "" {
				if ind := imports.indentFrom(m.sourceRelative, m.result); ind != "" {
					destLine = ind + destLine
				}
			}
		}
		edits = append(edits, imports.dropMemberFrom(m.sourceRelative, leaf, destLine, m.result)...)
	}
	return edits, nil
}

func (m moveQualify) dropHoleOnlyImports(fileExtract *project.FileExtract, source []byte) *project.FileExtract {
	if fileExtract == nil || m.result == nil || m.hole == nil || m.hole.RemoveEnd <= m.hole.RemoveStart {
		return fileExtract
	}
	out := *fileExtract
	out.Imports = nil
	for _, importDef := range fileExtract.Imports {
		if m.holeOnlyImport(importDef, source) {
			continue
		}
		out.Imports = append(out.Imports, importDef)
	}
	return &out
}

func (m moveQualify) holeOnlyImport(importDef project.ImportDef, source []byte) bool {
	locals := namedImportLocals(importDef, source)
	if len(locals) == 0 {
		return false
	}
	hole := *m.hole
	for _, local := range locals {
		if !containsIdentifier([]byte(hole.DeclText), local) {
			return false
		}
	}
	for _, local := range locals {
		for _, u := range m.result.Uses {
			uref := ParseReference(u.Reference)
			if strings.TrimPrefix(uref.Path, "./") != m.sourceRelative {
				continue
			}
			if u.StartByte >= hole.RemoveStart && u.EndByte <= hole.RemoveEnd {
				continue
			}
			if importDef.EndByte > importDef.StartByte && u.StartByte >= importDef.StartByte && u.EndByte <= importDef.EndByte {
				continue
			}
			if AtomName(uref.Name) == local || AtomName(u.Target) == local {
				return false
			}
		}
	}
	return true
}

func (m moveQualify) leftoverIdentifierEdits(source []byte, leaf, qualifier string) []project.Edit {
	if m.hole == nil || len(source) == 0 || leaf == "" || qualifier == "" {
		return nil
	}
	hole := *m.hole
	var edits []project.Edit
	walk := func(lo, hi int) {
		if lo < 0 {
			lo = 0
		}
		if hi > len(source) {
			hi = len(source)
		}
		for i := lo; i < hi; {
			if !isIdentByte(source[i]) || (i > 0 && isIdentByte(source[i-1])) {
				i++
				continue
			}
			j := i + 1
			for j < hi && isIdentByte(source[j]) {
				j++
			}
			if string(source[i:j]) == leaf && (i < 1 || source[i-1] != '.') {
				edits = append(edits, project.Edit{
					File:    m.sourceRelative,
					Span:    ingestutil.Span{StartByte: uint32(i), EndByte: uint32(j)},
					NewText: qualifier + "." + leaf,
				})
			}
			i = j
		}
	}
	walk(0, int(hole.RemoveStart))
	walk(int(hole.RemoveEnd), len(source))
	return edits
}

func (m moveQualify) holeDependencyBlock() string {
	edits := m.holeDependencyImports(nil)
	if len(edits) != 1 {
		return ""
	}
	return edits[0].NewText
}

func (m moveQualify) holeDependencyImports(destinationContent []byte) []project.Edit {
	if m.result == nil || m.hole == nil || m.hole.RemoveEnd <= m.hole.RemoveStart {
		return nil
	}
	info := m.pack.importLine
	result := m.result
	st := m.store
	sourceRelative := m.sourceRelative
	destinationRelative := m.destinationRelative
	dir := m.dir
	policy := m.policy
	hole := *m.hole
	if info.HasQual && len(destinationContent) > 0 {
		return nil
	}
	srcFE := fileExtractForImports(result, sourceRelative, st)
	if srcFE == nil || len(srcFE.Imports) == 0 {
		return nil
	}
	needed := map[string]bool{}
	text := hole.DeclText
	for _, u := range result.Uses {
		uref := ParseReference(u.Reference)
		if strings.TrimPrefix(uref.Path, "./") != sourceRelative {
			continue
		}
		if u.StartByte < hole.RemoveStart || u.EndByte > hole.RemoveEnd {
			continue
		}
		t := ParseReference(u.Target)
		if t.Name == "" {
			continue
		}
		needed[AtomName(t.Name)] = true
		if t.Path != "" {
			needed[strings.TrimPrefix(t.Path, "./")] = true
		}
	}
	for _, importDef := range srcFE.Imports {
		if local := importLocalName(importDef); local != "" && containsIdentifier([]byte(text), local) {
			needed[local] = true
		}
	}
	srcBytes, err := (projectfs.OS{}).ReadFile(path.Join(dir, sourceRelative))
	if err != nil {
		return nil
	}
	sourceImports := loadImportFile(srcBytes, srcFE)
	for _, binding := range sourceImports.bindings() {
		if containsIdentifier([]byte(text), binding.name) {
			needed[binding.name] = true
		}
	}
	if len(needed) == 0 {
		return nil
	}
	dstFE := fileExtractForImports(result, destinationRelative, st)
	var extras []string
	seen := map[string]bool{}
	for _, importDef := range srcFE.Imports {
		local := importLocalName(importDef)
		member := importDef.MemberName != ""
		if !needed[local] && !needed[importDef.MemberName] && (member || !needed[importDef.SourcePath]) {
			continue
		}
		if destinationFileHasAtomName(m.result, m.destinationRelative, local) || loadImportFile(destinationContent, dstFE).importsPath(importDef.SourcePath) {
			continue
		}
		if importSpecifier(importDef.SourcePath).namesFileFrom(sourceRelative, destinationRelative, nil) {
			continue
		}
		key := importDeduplicateKey(importDef)
		if seen[key] {
			continue
		}
		if chunk := destinationImportText(m.pack.importLine, m.sourceRelative, m.destinationRelative, m.dir, m.policy, importDef, srcBytes); chunk != "" {
			seen[key] = true
			extras = append(extras, chunk)
		}
	}
	if info.HasLeaf {
		for _, binding := range sourceImports.bindings() {
			if !needed[binding.name] {
				continue
			}
			key := strings.Trim(binding.module, `"'`) + "\t" + binding.name
			if seen[key] {
				continue
			}
			if line := destinationImportLine(info, binding.module, binding.name, binding.alias); line != "" {
				seen[key] = true
				extras = append(extras, strings.TrimRight(line, "\n"))
			}
		}
	}
	if len(extras) == 0 {
		return nil
	}
	if at, lines := destinationImportGroupInsert(destinationContent, dstFE, extras, info); lines != "" {
		return []project.Edit{{
			File:    destinationRelative,
			Span:    ingestutil.Span{StartByte: at, EndByte: at},
			NewText: lines,
		}}
	}
	joined := wrapCopiedImports(extras, info)
	if e, ok := rewriteLayoutUnit(store.RelationImport, destinationRelative, destinationContent, dstFE, result, policy, joined); ok {
		return []project.Edit{e}
	}
	return nil
}

func (m moveQualify) leftoverFromImports(left map[string]bool, destinationContent []byte) []project.Edit {
	specifier := destinationRelativeSpecifier(m.destinationRelative, m.sourceRelative, m.pack.importLine, m.dir, m.policy)
	if specifier == "" {
		return nil
	}
	names := make([]string, 0, len(left))
	for n := range left {
		if n == "" || destinationFileHasAtomName(m.result, m.destinationRelative, n) {
			continue
		}
		names = append(names, n)
	}
	if len(names) == 0 {
		return nil
	}
	slices.Sort(names)
	var b strings.Builder
	for _, n := range names {
		b.WriteString(m.pack.emitImportLine(specifier, n, ""))
	}
	fileExtract := fileExtractForImports(m.result, m.destinationRelative, nil)
	text := b.String()
	if e, ok := rewriteLayoutUnit(store.RelationImport, m.destinationRelative, destinationContent, fileExtract, m.result, m.policy, text); ok {
		return []project.Edit{e}
	}
	return nil
}

func (m moveQualify) retargetLeafFrom(ctx context.Context) ([]project.Edit, error) {
	leaf := AtomName(m.source.Name)
	if m.result == nil || leaf == "" {
		return nil, nil
	}
	var edits []project.Edit
	seen := map[string]bool{}
	add := func(ref string) {
		f := strings.TrimPrefix(ParseReference(ref).Path, "./")
		if f == "" || f == m.sourceRelative || f == m.destinationRelative {
			return
		}
		seen[f] = true
	}
	for _, a := range m.result.Aliases {
		add(a.Reference)
	}
	for _, u := range m.result.Uses {
		add(u.Reference)
	}
	for f := range seen {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !fileKeepsOldPackage(m.result, f, m.sourceRelative, m.entity) {
			continue
		}
		content, err := (projectfs.OS{}).ReadFile(path.Join(m.dir, f))
		if err != nil {
			continue
		}
		for _, site := range loadImportFile(content, fileExtractForImports(m.result, f, nil)).sites {
			if !specifierNamesMovedSource(site.specifier, f, m.sourceRelative, m.result) || !site.onlyLeaf(leaf) {
				continue
			}
			neu := destinationRelativeSpecifier(f, m.destinationRelative, m.pack.importLine, m.dir, m.policy)
			if neu == "" || neu == site.specifier {
				continue
			}
			stmt := string(content[site.start:site.end])
			at := strings.Index(stmt, site.specifier)
			if at < 0 {
				continue
			}
			edits = append(edits, project.Edit{
				File: f,
				Span: ingestutil.Span{
					StartByte: site.start + uint32(at),
					EndByte:   site.start + uint32(at+len(site.specifier)),
				},
				NewText: neu,
			})
		}
	}
	return edits, nil
}

func (m moveQualify) holeLeftover(destinationContent []byte) []project.Edit {
	if m.result == nil || m.hole == nil || !m.pack.importLineOkay() {
		return nil
	}
	left := leftoverSourceNames(m.result, m.sourceRelative, m.entity, *m.hole)
	if len(left) == 0 {
		return nil
	}
	if m.pack.importLine.HasLeaf {
		return m.leftoverFromImports(left, destinationContent)
	}
	if m.pack.importLine.HasQual && len(destinationContent) == 0 {
		return m.leftoverQualifier(destinationContent)
	}
	return nil
}

func (m moveQualify) leftoverQualifier(destinationContent []byte) []project.Edit {
	specifier := destinationRelativeSpecifier(m.destinationRelative, m.sourceRelative, m.pack.importLine, m.dir, m.policy)
	if specifier == "" {
		return nil
	}
	line := m.pack.emitImportLine(specifier, "", fileStem(m.sourceRelative))
	if line == "" {
		return nil
	}
	fileExtract := fileExtractForImports(m.result, m.destinationRelative, nil)
	if e, ok := rewriteLayoutUnit(store.RelationImport, m.destinationRelative, destinationContent, fileExtract, m.result, m.policy, line); ok {
		return []project.Edit{e}
	}
	return nil
}

func (m moveQualify) qualifySelectorEdit(fileRel string, source []byte, start, end uint32, destinationQualifier string) (project.Edit, bool) {
	if qs, qe, ok := qualifierBefore(source, start); ok {
		if string(source[qs:qe]) == destinationQualifier {
			return project.Edit{}, true
		}
		return project.Edit{
			File:    fileRel,
			Span:    ingestutil.Span{StartByte: qs, EndByte: qe},
			NewText: destinationQualifier,
		}, true
	}
	if int(end) > len(source) || end <= start {
		return project.Edit{}, false
	}
	qs, qe, ok := qualifierInSpan(source[start:end])
	if !ok {
		return project.Edit{}, false
	}
	if string(source[start+qs:start+qe]) == destinationQualifier {
		return project.Edit{}, true
	}
	return project.Edit{
		File:    fileRel,
		Span:    ingestutil.Span{StartByte: start + qs, EndByte: start + qe},
		NewText: destinationQualifier,
	}, true
}

func qualifierInSpan(seg []byte) (uint32, uint32, bool) {
	for i := 0; i < len(seg); i++ {
		if seg[i] == '.' {
			if i == 0 || !isIdentByte(seg[i-1]) {
				return 0, 0, false
			}
			return 0, uint32(i), true
		}
		if i+1 < len(seg) && seg[i] == ':' && seg[i+1] == ':' {
			if i == 0 || !isIdentByte(seg[i-1]) {
				return 0, 0, false
			}
			return 0, uint32(i), true
		}
		if !isIdentByte(seg[i]) {
			return 0, 0, false
		}
	}
	return 0, 0, false
}

// qualifierBefore reports the identifier immediately before a `.` or `::` join
// at start (the use leaf).
func qualifierBefore(source []byte, start uint32) (uint32, uint32, bool) {
	if int(start) > len(source) {
		return 0, 0, false
	}
	i := int(start)
	for i > 0 && (source[i-1] == ' ' || source[i-1] == '\t') {
		i--
	}
	if i > 0 && source[i-1] == '.' {
		i--
	} else if i > 1 && source[i-1] == ':' && source[i-2] == ':' {
		i -= 2
	} else {
		return 0, 0, false
	}
	end := uint32(i)
	for i > 0 && isIdentByte(source[i-1]) {
		i--
	}
	if uint32(i) == end {
		return 0, 0, false
	}
	return uint32(i), end, true
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}
