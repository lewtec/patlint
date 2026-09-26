package ingest

import (
	"os"
	"path"
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// importFile is every import-relation statement in one file (Composite).
type importFile struct {
	path    string
	source  []byte
	extract *project.FileExtract
	sites   []importSite
}

// importSite is one import statement. Members are import-relation name-field rows.
type importSite struct {
	file      string
	start     uint32
	end       uint32
	specifier string
	from      bool
	members   []importMember
}

type importMember struct {
	name, alias string
	start, end  uint32
}

type importBinding struct {
	module, name, alias string
}

func loadImportFile(source []byte, extract *project.FileExtract) importFile {
	file := importFile{source: source, extract: extract}
	if extract != nil {
		file.path = strings.TrimPrefix(extract.Path, "./")
	}
	file.sites = collectImportSites(source, extract)
	return file
}

func collectImportSites(source []byte, extract *project.FileExtract) []importSite {
	if len(source) == 0 || extract == nil {
		return nil
	}
	var sites []importSite
	eachImportStatement(source, extract, func(start, end uint32, def project.ImportDef) bool {
		members := membersInSpan(extract, start, end)
		sites = append(sites, importSite{
			file:      strings.TrimPrefix(extract.Path, "./"),
			start:     start,
			end:       end,
			specifier: importStatementSpecifier(def, source[start:end]),
			from:      len(members) > 0,
			members:   members,
		})
		return true
	})
	return sites
}

func membersInSpan(extract *project.FileExtract, start, end uint32) []importMember {
	if extract == nil {
		return nil
	}
	var members []importMember
	seen := map[string]bool{}
	for _, def := range extract.Imports {
		if def.MemberName == "" {
			continue
		}
		pos := def.StartByte
		if pos < start || pos >= end {
			if def.TargetStartByte < start || def.TargetStartByte >= end {
				continue
			}
			pos = def.TargetStartByte
		}
		alias := ""
		if def.HasAliasBinding && def.LocalName != "" && def.LocalName != def.MemberName {
			alias = def.LocalName
		}
		key := def.MemberName + "\t" + alias
		if seen[key] {
			continue
		}
		seen[key] = true
		memberEnd := def.EndByte
		if memberEnd <= pos {
			memberEnd = pos + uint32(len(def.MemberName))
		}
		members = append(members, importMember{name: def.MemberName, alias: alias, start: pos, end: memberEnd})
	}
	return members
}

func (file importFile) importsPath(fileRel string) bool {
	if file.extract == nil {
		return false
	}
	for _, importDef := range file.extract.Imports {
		if importSpecifier(importDef.SourcePath).namesFile(fileRel) {
			return true
		}
	}
	return false
}

func (file importFile) importsResolved(importerRel, fileRel string, policy PackQueries) bool {
	if file.extract == nil {
		return false
	}
	for _, importDef := range file.extract.Imports {
		resolved := importSpecifier(importDef.SourcePath).resolveFrom(importerRel)
		if resolved != "" && (importSpecifier(resolved).namesFile(fileRel) || specifierNamesIndex(resolved, fileRel, policy)) {
			return true
		}
		if resolved == "" && importSpecifier(importDef.SourcePath).namesFile(fileRel) {
			return true
		}
	}
	return false
}

func (file importFile) hasOnlyMember(leaf string) bool {
	for _, site := range file.sites {
		if site.onlyLeaf(leaf) {
			return true
		}
	}
	return false
}

func (file importFile) hasOtherMembers(leaf string) bool {
	for _, site := range file.sites {
		if site.keepsOther(leaf) {
			return true
		}
	}
	return false
}

func (file importFile) hasOtherMembersFrom(sourceRel, leaf string, result *project.Result) bool {
	for _, site := range file.sites {
		if specifierNamesMovedSource(site.specifier, file.path, sourceRel, result) && site.keepsOther(leaf) {
			return true
		}
	}
	return false
}

func (file importFile) aliasFor(sourceRel, leaf string) string {
	for _, site := range file.sites {
		if !importSpecifier(site.specifier).namesFile(sourceRel) && !importSpecifier(site.specifier).namesFileFrom(file.path, sourceRel, nil) {
			continue
		}
		if alias := site.aliasOf(leaf); alias != "" || site.onlyLeaf(leaf) {
			return alias
		}
	}
	return ""
}

func (file importFile) bindings() []importBinding {
	var out []importBinding
	for _, site := range file.sites {
		out = append(out, site.bindings()...)
	}
	return out
}

func (file importFile) moduleFrom(sourceRel string, result *project.Result) string {
	for _, site := range file.sites {
		if specifierNamesMovedSource(site.specifier, file.path, sourceRel, result) {
			return site.specifier
		}
	}
	return ""
}

func (file importFile) indentFrom(sourceRel string, result *project.Result) string {
	for _, site := range file.sites {
		if !specifierNamesMovedSource(site.specifier, file.path, sourceRel, result) {
			continue
		}
		line := string(file.source[site.start:site.end])
		return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	}
	return ""
}

func (file importFile) dropMemberFrom(sourceRel, leaf, destinationLine string, result *project.Result) []project.Edit {
	if leaf == "" || len(file.source) == 0 {
		return nil
	}
	var edits []project.Edit
	for _, site := range file.sites {
		if !specifierNamesMovedSource(site.specifier, file.path, sourceRel, result) {
			continue
		}
		if site.file == "" {
			site.file = file.path
		}
		if edit, ok := site.dropMember(file.source, leaf, destinationLine); ok {
			edits = append(edits, edit)
		}
	}
	return edits
}

func (file importFile) rewriteMember(packageName, oldStem, newStem string) []project.Edit {
	if oldStem == "" || newStem == "" || oldStem == newStem || len(file.source) == 0 {
		return nil
	}
	var edits []project.Edit
	for _, site := range file.sites {
		if site.specifier != packageName && site.specifier != "." {
			continue
		}
		if edit, ok := site.rewriteMember(file.source, oldStem, newStem); ok {
			if edit.File == "" {
				edit.File = file.path
			}
			edits = append(edits, edit)
		}
	}
	return edits
}

func (site importSite) onlyLeaf(leaf string) bool {
	return leaf != "" && len(site.members) == 1 && site.members[0].name == leaf
}

func (site importSite) keepsOther(leaf string) bool {
	if leaf == "" || len(site.members) < 2 {
		return false
	}
	for _, member := range site.members {
		if member.name == leaf {
			return true
		}
	}
	return false
}

func (site importSite) aliasOf(leaf string) string {
	for _, member := range site.members {
		if member.name == leaf {
			return member.alias
		}
	}
	return ""
}

func (site importSite) bindings() []importBinding {
	if !site.from || site.specifier == "" {
		return nil
	}
	var out []importBinding
	for _, member := range site.members {
		if member.name == "" {
			continue
		}
		out = append(out, importBinding{module: site.specifier, name: member.name, alias: member.alias})
	}
	return out
}

func (site importSite) dropMember(source []byte, leaf, destinationLine string) (project.Edit, bool) {
	if int(site.end) > len(source) || site.end <= site.start || leaf == "" {
		return project.Edit{}, false
	}
	var dropped importMember
	found := false
	keep := 0
	for _, member := range site.members {
		if member.name == leaf {
			dropped = member
			found = true
			continue
		}
		keep++
	}
	if !found || keep == 0 || dropped.end <= dropped.start || int(dropped.end) > len(source) {
		return project.Edit{}, false
	}
	dropStart, dropEnd := commaAround(source, dropped.start, dropped.end)
	if dropStart < site.start {
		dropStart = site.start
	}
	if dropEnd > site.end {
		dropEnd = site.end
	}
	text := string(source[site.start:dropStart]) + string(source[dropEnd:site.end])
	end := site.end
	if destinationLine != "" {
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += destinationLine
		if int(end) < len(source) && source[end] == '\n' && (end == 0 || source[end-1] != '\n') {
			end++
		}
	}
	return project.Edit{
		File:    strings.TrimPrefix(site.file, "./"),
		Span:    ingestutil.Span{StartByte: site.start, EndByte: end},
		NewText: text,
	}, true
}

func (site importSite) rewriteMember(source []byte, oldStem, newStem string) (project.Edit, bool) {
	if oldStem == "" || newStem == "" || oldStem == newStem {
		return project.Edit{}, false
	}
	for _, member := range site.members {
		if member.name != oldStem || member.end <= member.start || int(member.end) > len(source) {
			continue
		}
		end := member.end
		text := newStem
		if member.alias == "" {
			text = newStem + " as " + oldStem
		} else {
			end = member.start + uint32(len(member.name))
			if end > member.end {
				end = member.end
			}
		}
		return project.Edit{
			File:    strings.TrimPrefix(site.file, "./"),
			Span:    ingestutil.Span{StartByte: member.start, EndByte: end},
			NewText: text,
		}, true
	}
	return project.Edit{}, false
}

func commaAround(source []byte, start, end uint32) (uint32, uint32) {
	i := int(end)
	for i < len(source) && (source[i] == ' ' || source[i] == '\t') {
		i++
	}
	if i < len(source) && source[i] == ',' {
		i++
		for i < len(source) && (source[i] == ' ' || source[i] == '\t') {
			i++
		}
		return start, uint32(i)
	}
	j := int(start)
	for j > 0 && (source[j-1] == ' ' || source[j-1] == '\t') {
		j--
	}
	if j > 0 && source[j-1] == ',' {
		j--
		for j > 0 && (source[j-1] == ' ' || source[j-1] == '\t') {
			j--
		}
		return uint32(j), end
	}
	return start, end
}

func importKeepsOtherMembers(dir string, result *project.Result, file, specifier, sourceRelative, leaf string) bool {
	if leaf == "" {
		return false
	}
	content, err := os.ReadFile(path.Join(dir, strings.TrimPrefix(file, "./")))
	if err != nil || len(content) == 0 {
		return false
	}
	_ = specifier
	_ = sourceRelative
	return loadImportFile(content, fileExtractForImports(result, file, nil)).hasOtherMembers(leaf)
}

func rewriteFromPackageModuleImport(fileRel string, src []byte, packageName, oldStem, newStem string, result *project.Result) []project.Edit {
	file := loadImportFile(src, fileExtractForImports(result, fileRel, nil))
	file.path = strings.TrimPrefix(fileRel, "./")
	return file.rewriteMember(packageName, oldStem, newStem)
}
