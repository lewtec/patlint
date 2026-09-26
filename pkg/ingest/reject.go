package ingest

import (
	"fmt"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// rejectMovedAtom reports why a cross-file hole must not be applied.
func rejectMovedAtom(dir string, result *project.Result, src, dst Reference, entity project.Atom, hole DeclExtract, policy PackQueries) error {
	sourceRelative := strings.TrimPrefix(src.Path, "./")
	destinationRelative := strings.TrimPrefix(dst.Path, "./")
	if sourceRelative == destinationRelative {
		return nil
	}
	leaf := AtomName(src.Name)
	if leaf == "" {
		return nil
	}
	if nestedMemberInScope(result, sourceRelative, entity) {
		return fmt.Errorf("cross-file move of struct field is not supported")
	}
	if iotaConstantGroup(dir, result, sourceRelative, entity) {
		return fmt.Errorf("cross-file move of const using iota in a group is not supported")
	}
	sourceDirectory := importFileDir(sourceRelative)
	dstDir := importFileDir(destinationRelative)
	if sourceDirectory == dstDir {
		return nil
	}
	if !languageRules(policy, languageForRefPath(result, src.Path)).PackageScopedBareNames {
		return nil
	}
	if leaf != "init" && !identifierExported(leaf) {
		return fmt.Errorf("cross-file move of unexported symbol %s is not supported", leaf)
	}
	if testFile(sourceRelative) && !testFile(destinationRelative) && strings.HasPrefix(leaf, "Test") {
		return fmt.Errorf("moving test function %s into non-test file %s is not supported", leaf, destinationRelative)
	}
	if recv := memberReceiver(ParseReference(entity.Reference).Name); recv == "" &&
		strings.HasPrefix(strings.TrimSpace(hole.DeclText), "type ") {
		if m := packageMemberOf(result, sourceDirectory, leaf); m != "" {
			return fmt.Errorf("cross-file move of type %s with methods in package is not supported", leaf)
		}
	}
	if dep := holeSameModuleDependency(result, sourceRelative, sourceDirectory, entity, hole); dep != "" {
		return fmt.Errorf("cross-file move of %s still depends on same-package symbol %s is not supported", leaf, dep)
	}
	return nil
}

// rejectCircularResidualMove refuses a cross-file move that would import dest
// from source (residual uses) and source from dest (hole deps or dest already
// imports source).
func rejectCircularResidualMove(dir string, result *project.Result, src, dst Reference, entity project.Atom, hole DeclExtract) error {
	sourceRelative := strings.TrimPrefix(src.Path, "./")
	destinationRelative := strings.TrimPrefix(dst.Path, "./")
	if sourceRelative == destinationRelative || result == nil {
		return nil
	}
	leaf := AtomName(src.Name)
	residual := fileUsesMovedOutside(result, sourceRelative, src, entity, hole)
	if !residual {
		srcBytes, err := os.ReadFile(path.Join(dir, sourceRelative))
		if err == nil && leaf != "" {
			residual = textHasIdentOutside(srcBytes, leaf, hole.RemoveStart, hole.RemoveEnd)
		}
	}
	if !residual {
		return nil
	}
	deps := LocalDepsInDeclSpan(result, src, hole, LocalDepOpts{TopLevelOnly: true})
	if len(deps) == 0 {
		deps = textLocalDeps(hole.DeclText, remainingTopLevelNames(result, sourceRelative, entity, hole))
	}
	if len(deps) > 0 {
		return fmt.Errorf("unsupported: moving %s from %s to %s would create a circular import (residual reverse import + local deps %v)",
			leaf, sourceRelative, destinationRelative, deps)
	}
	if fileImportsFile(result, destinationRelative, sourceRelative) {
		return fmt.Errorf("unsupported: moving %s from %s to %s would create a circular import (destination already imports source; residual reverse import)",
			leaf, sourceRelative, destinationRelative)
	}
	return nil
}

func fileUsesMovedOutside(result *project.Result, fileRel string, src Reference, entity project.Atom, hole DeclExtract) bool {
	fileRel = strings.TrimPrefix(fileRel, "./")
	for _, u := range result.Uses {
		if strings.TrimPrefix(ParseReference(u.Reference).Path, "./") != fileRel {
			continue
		}
		if u.EndByte > hole.RemoveStart && u.StartByte < hole.RemoveEnd {
			continue
		}
		if useTargetsMovedAtom(u, src, entity) {
			return true
		}
	}
	return false
}

func textHasIdentOutside(src []byte, ident string, start, end uint32) bool {
	if ident == "" || len(src) == 0 {
		return false
	}
	if int(end) > len(src) {
		end = uint32(len(src))
	}
	if start > end {
		return ingestutil.IdentUsed(string(src), ident, ingestutil.IsIdentChar)
	}
	var b strings.Builder
	b.Write(src[:start])
	b.Write(src[end:])
	return ingestutil.IdentUsed(b.String(), ident, ingestutil.IsIdentChar)
}

func remainingTopLevelNames(result *project.Result, fileRel string, entity project.Atom, hole DeclExtract) []string {
	fileRel = strings.TrimPrefix(fileRel, "./")
	moved := AtomName(ParseReference(entity.Reference).Name)
	var out []string
	seen := map[string]bool{}
	for _, a := range result.Atoms {
		if a.Reference == entity.Reference {
			continue
		}
		r := ParseReference(a.Reference)
		if strings.TrimPrefix(r.Path, "./") != fileRel || strings.Contains(r.Name, ".") {
			continue
		}
		if hole.RemoveEnd > hole.RemoveStart && a.StartByte >= hole.RemoveStart && a.EndByte <= hole.RemoveEnd {
			continue
		}
		n := AtomName(r.Name)
		if n == "" || n == moved || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func textLocalDeps(declText string, names []string) []string {
	var out []string
	for _, n := range names {
		if ingestutil.IdentUsed(declText, n, ingestutil.IsIdentChar) {
			out = append(out, n)
		}
	}
	return out
}

func fileImportsFile(result *project.Result, fromRel, toRel string) bool {
	fromRel = strings.TrimPrefix(fromRel, "./")
	toRel = strings.TrimPrefix(toRel, "./")
	if result == nil || fromRel == "" || toRel == "" {
		return false
	}
	toDir := importFileDir(toRel)
	toStem := fileStem(toRel)
	for _, a := range result.Aliases {
		if strings.TrimPrefix(ParseReference(a.Reference).Path, "./") != fromRel {
			continue
		}
		t := ParseReference(a.Target)
		tp := strings.TrimPrefix(t.Path, "./")
		if importSpecifier(tp).namesFile(toRel) || tp == toRel || importFileDir(tp) == toDir {
			return true
		}
		if toStem != "" && (tp == toStem || strings.HasSuffix(tp, "."+toStem)) {
			return true
		}
	}
	return false
}

func nestedMemberInScope(result *project.Result, fileRelative string, entity project.Atom) bool {
	if result == nil {
		return false
	}
	qual := atomQualifier(ParseReference(entity.Reference).Name)
	if qual == "" {
		return false
	}
	sc := coveringScope(result, fileRelative, entity.StartByte, entity.EndByte)
	if sc == nil {
		return false
	}
	for _, a := range result.Atoms {
		r := ParseReference(a.Reference)
		if strings.TrimPrefix(r.Path, "./") != fileRelative {
			continue
		}
		if r.Name != qual {
			continue
		}
		if a.StartByte >= sc.StartByte && a.EndByte <= sc.EndByte {
			return true
		}
	}
	return false
}

func iotaConstantGroup(dir string, result *project.Result, fileRelative string, entity project.Atom) bool {
	sc := coveringScope(result, fileRelative, entity.StartByte, entity.EndByte)
	for p := sc; p != nil; p = scopeAt(result, fileRelative, p.Parent) {
		if !p.HoleOnly {
			continue
		}
		src, err := os.ReadFile(path.Join(dir, fileRelative))
		if err != nil || int(p.EndByte) > len(src) || p.EndByte <= p.StartByte {
			return false
		}
		if firstIdentifierToken(src, p.StartByte, p.EndByte) != "const" {
			continue
		}
		return containsIdentifier(src[p.StartByte:p.EndByte], "iota")
	}
	return false
}

func packageMemberOf(result *project.Result, packageDirectory, typeName string) string {
	if result == nil || typeName == "" {
		return ""
	}
	for _, a := range result.Atoms {
		r := ParseReference(a.Reference)
		if importFileDir(r.Path) != packageDirectory {
			continue
		}
		if memberReceiver(r.Name) == typeName {
			return r.Name
		}
	}
	return ""
}

func holeSameModuleDependency(result *project.Result, sourceRelative, sourceDirectory string, entity project.Atom, hole DeclExtract) string {
	if result == nil {
		return ""
	}
	self := ParseReference(entity.Reference).Name
	for _, u := range result.Uses {
		uref := ParseReference(u.Reference)
		if strings.TrimPrefix(uref.Path, "./") != sourceRelative {
			continue
		}
		if u.StartByte < hole.RemoveStart || u.EndByte > hole.RemoveEnd {
			continue
		}
		t := ParseReference(u.Target)
		if t.Name == "" || AtomName(t.Name) == AtomName(self) {
			continue
		}
		if !targetInModule(t, sourceDirectory) {
			continue
		}
		if useAtomInsideHole(result, u.Target, sourceRelative, hole) {
			continue
		}
		return AtomName(t.Name)
	}
	return ""
}

func useAtomInsideHole(result *project.Result, target, sourceRelative string, hole DeclExtract) bool {
	for _, a := range result.Atoms {
		if a.Reference != target {
			continue
		}
		r := ParseReference(a.Reference)
		if strings.TrimPrefix(r.Path, "./") != sourceRelative {
			return false
		}
		return a.StartByte >= hole.RemoveStart && a.EndByte <= hole.RemoveEnd
	}
	return false
}

func targetInModule(t Reference, sourceDirectory string) bool {
	if t.Provider != "path" && t.Provider != "" {
		p := strings.ReplaceAll(t.Path, ".", "/")
		return sourceDirectory != "" && (p == sourceDirectory || strings.HasSuffix(p, "/"+sourceDirectory))
	}
	return importFileDir(t.Path) == sourceDirectory
}

func memberReceiver(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	recv := strings.TrimPrefix(name[:i], "*")
	if j := strings.IndexByte(recv, '['); j >= 0 {
		recv = recv[:j]
	}
	return recv
}

func atomQualifier(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	return name[:i]
}

func identifierExported(leaf string) bool {
	r, _ := utf8.DecodeRuneInString(leaf)
	return unicode.IsUpper(r)
}

func testFile(fileRelative string) bool {
	base := path.Base(fileRelative)
	ext := path.Ext(base)
	return strings.HasSuffix(strings.TrimSuffix(base, ext), "_test")
}

func containsIdentifier(src []byte, word string) bool {
	if word == "" {
		return false
	}
	for i := 0; i+len(word) <= len(src); i++ {
		if string(src[i:i+len(word)]) != word {
			continue
		}
		if i > 0 && isIdentByte(src[i-1]) {
			continue
		}
		if i+len(word) < len(src) && isIdentByte(src[i+len(word)]) {
			continue
		}
		return true
	}
	return false
}
