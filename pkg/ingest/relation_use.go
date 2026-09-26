package ingest

import (
	"path"
	"strings"

	"github.com/lewtec/patlint/pkg/project"
)

// Use-relation leftover queries. Mechanism. Apply and rename call these;
// they do not write edit-relation rows.
//
//	useTargetsMovedAtom   — this use names the moved atom
//	fileKeepsOldPackage   — file still uses another atom from the old package
//	leftoverSourceNames   — names used in the hole that stay in src
//	qualifyIdentifiersInText   — prefix leftover names in copied text
//	destinationFileHasAtomName — dest file already has this atom name

func destinationFileHasAtomName(result *project.Result, destinationRelative, name string) bool {
	if result == nil || name == "" {
		return false
	}
	destinationRelative = strings.TrimPrefix(destinationRelative, "./")
	for _, a := range result.Atoms {
		if strings.TrimPrefix(ParseReference(a.Reference).Path, "./") != destinationRelative {
			continue
		}
		if AtomName(ParseReference(a.Reference).Name) == name {
			return true
		}
	}
	return false
}

func useTargetsMovedAtom(u project.Use, src Reference, entity project.Atom) bool {
	if u.Target == entity.Reference {
		return true
	}
	t := ParseReference(u.Target)
	if AtomName(t.Name) != AtomName(src.Name) {
		return false
	}
	srcDir := importFileDir(src.Path)
	if t.Provider == "path" || t.Provider == "" {
		return importFileDir(t.Path) == srcDir
	}
	if srcDir == "" {
		return false
	}
	p := strings.ReplaceAll(t.Path, ".", "/")
	return p == srcDir || strings.HasSuffix(p, "/"+srcDir)
}

func fileKeepsOldPackage(result *project.Result, fileRel, sourceRelative string, moved project.Atom) bool {
	sourceRelative = strings.TrimPrefix(sourceRelative, "./")
	if result == nil || sourceRelative == "" {
		return false
	}
	fileRel = strings.TrimPrefix(fileRel, "./")
	srcDir := importFileDir(sourceRelative)
	fileMod := path.Ext(sourceRelative) != ""
	for _, u := range result.Uses {
		if u.Target == moved.Reference {
			continue
		}
		uref := ParseReference(u.Reference)
		if strings.TrimPrefix(uref.Path, "./") != fileRel {
			continue
		}
		t := ParseReference(u.Target)
		if t.Name == "" {
			continue
		}
		movedLeaf := ParseReference(moved.Reference).Name
		if AtomName(t.Name) == movedLeaf {
			continue
		}
		if t.Provider != "path" && t.Provider != "" {
			p := strings.ReplaceAll(t.Path, ".", "/")
			if srcDir != "" && (p == srcDir || strings.HasSuffix(p, "/"+srcDir)) {
				return true
			}
			continue
		}
		if fileMod {
			if importSpecifier(t.Path).namesFile(sourceRelative) {
				return true
			}
			continue
		}
		if importFileDir(t.Path) == srcDir {
			return true
		}
	}
	return false
}

func leftoverSourceNames(result *project.Result, sourceRelative string, entity project.Atom, hole DeclExtract) map[string]bool {
	left := map[string]bool{}
	if result == nil {
		return left
	}
	moved := AtomName(ParseReference(entity.Reference).Name)
	remain := map[string]bool{}
	for _, a := range result.Atoms {
		if a.Reference == entity.Reference {
			continue
		}
		er := ParseReference(a.Reference)
		if strings.TrimPrefix(er.Path, "./") != sourceRelative {
			continue
		}
		if hole.RemoveEnd > hole.RemoveStart && a.StartByte >= hole.RemoveStart && a.EndByte <= hole.RemoveEnd {
			continue
		}
		n := AtomName(er.Name)
		if n == "" || n == moved {
			continue
		}
		remain[a.Reference] = true
	}
	for _, u := range result.Uses {
		if !remain[u.Target] {
			continue
		}
		uref := ParseReference(u.Reference)
		if strings.TrimPrefix(uref.Path, "./") != sourceRelative {
			continue
		}
		if u.StartByte < hole.RemoveStart || u.EndByte > hole.RemoveEnd {
			continue
		}
		n := AtomName(ParseReference(u.Target).Name)
		if n != "" {
			left[n] = true
		}
	}
	return left
}

func qualifyIdentifiersInText(text string, names map[string]bool, qual string) string {
	if qual == "" || len(names) == 0 {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); {
		if !isIdentByte(text[i]) || (i > 0 && isIdentByte(text[i-1])) {
			b.WriteByte(text[i])
			i++
			continue
		}
		j := i + 1
		for j < len(text) && isIdentByte(text[j]) {
			j++
		}
		word := text[i:j]
		if names[word] && (i < 2 || text[i-1] != '.') {
			b.WriteString(qual)
			b.WriteByte('.')
		}
		b.WriteString(word)
		i = j
	}
	return b.String()
}
