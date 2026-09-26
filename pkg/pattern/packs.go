package pattern

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// PackFile is one .rft source (name is for errors only).
type PackFile struct {
	Name string
	Src  string
}

// Packs is a loaded set of .rft files (prelude or any extra tree).
// Query it for host language and as-family claims. LispVM.New compiles
// Sources into one Packs; there is no process-global product.
type Packs struct {
	prog  *ExtractProgram
	env   expandEnv
	files []PackFile
}

// packFilesFS reads every *.rft under dir in fsys (sorted).
func packFilesFS(fsys fs.FS, dir string) ([]PackFile, error) {
	if fsys == nil {
		return nil, fmt.Errorf("%w: nil fs", ErrExtract)
	}
	if dir == "" {
		dir = "."
	}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".rft") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	files := make([]PackFile, 0, len(names))
	for _, name := range names {
		fpath := name
		if dir != "." {
			fpath = path.Join(dir, name)
		}
		data, err := fs.ReadFile(fsys, fpath)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrExtract, fpath, err)
		}
		files = append(files, PackFile{Name: fpath, Src: string(data)})
	}
	return files, nil
}

// LoadFS reads every *.rft under dir in fsys (sorted), merges defs then extract.
func LoadFS(fsys fs.FS, dir string) (*Packs, error) {
	files, err := packFilesFS(fsys, dir)
	if err != nil {
		return nil, err
	}
	return LoadFiles(files)
}

// LoadFiles merges pack sources: defs first, then extract programs.
func LoadFiles(files []PackFile) (*Packs, error) {
	env := NewExpandEnv()
	for _, f := range files {
		forms, err := ParseSexpDataFile(f.Src)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrExtract, f.Name, err)
		}
		for i, raw := range forms {
			form, ok := raw.([]any)
			if !ok || len(form) == 0 {
				return nil, fmt.Errorf("%w: %s: form %d: want list", ErrExtract, f.Name, i)
			}
			h, _ := form[0].(string)
			if h != "def" {
				continue
			}
			n, fn, err := ParseDef(form[1:])
			if err != nil {
				return nil, fmt.Errorf("%w: %s: def: %v", ErrExtract, f.Name, err)
			}
			env.Set(n, fn)
		}
	}
	if len(env) > 0 {
		SetExpandPrelude(env)
	}
	merged := &ExtractProgram{Path: "packs"}
	for _, f := range files {
		prog, err := loadExtractPack(f.Name, f.Src, env)
		if err != nil {
			return nil, err
		}
		merged.Actions = append(merged.Actions, prog.Actions...)
		merged.Families = append(merged.Families, prog.Families...)
		merged.Grammars = append(merged.Grammars, prog.Grammars...)
		merged.ImportLines = append(merged.ImportLines, prog.ImportLines...)
		merged.PackageLines = append(merged.PackageLines, prog.PackageLines...)
		merged.AtomicSpans = append(merged.AtomicSpans, prog.AtomicSpans...)
		merged.Docstrings = append(merged.Docstrings, prog.Docstrings...)
		merged.Layouts = append(merged.Layouts, prog.Layouts...)
		merged.DirectoryRepresentants = appendDirectoryRepresentants(merged.DirectoryRepresentants, prog.DirectoryRepresentants)
		if merged.Language == "" && prog.Language != "" {
			merged.Language = prog.Language
		}
	}
	stored := make([]PackFile, len(files))
	copy(stored, files)
	return &Packs{prog: merged, env: env, files: stored}, nil
}

// Program is the merged extract program.
func (p *Packs) Program() *ExtractProgram {
	if p == nil {
		return nil
	}
	return p.prog
}

// Families is the pack as-family claims (lang → family).
func (p *Packs) Families() []project.FamilyClaim {
	if p == nil || p.prog == nil {
		return nil
	}
	return p.prog.Families
}

// DirectoryRepresentants is the pack as-directory-representant path globs.
func (p *Packs) DirectoryRepresentants() []string {
	if p == nil || p.prog == nil {
		return nil
	}
	return p.prog.DirectoryRepresentants
}

func appendDirectoryRepresentants(dst, src []string) []string {
	seen := map[string]bool{}
	for _, g := range dst {
		seen[g] = true
	}
	for _, g := range src {
		if g == "" || seen[g] {
			continue
		}
		seen[g] = true
		dst = append(dst, g)
	}
	return dst
}

// RulesForLanguage is compiled as-family knobs for lang.
func (p *Packs) RulesForLanguage(lang string) project.LanguageRules {
	if p == nil {
		return project.LanguageRules{}
	}
	return rulesForLang(p.Families(), lang)
}

func rulesForLang(claims []project.FamilyClaim, lang string) project.LanguageRules {
	if lang == "" {
		return project.LanguageRules{}
	}
	var fam string
	for _, c := range claims {
		if c.Lang == lang {
			fam = c.Family
			break
		}
	}
	if fam == "" {
		return project.LanguageRules{}
	}
	var r project.LanguageRules
	for _, c := range claims {
		if c.Family != fam {
			continue
		}
		r.DirectoryModule = r.DirectoryModule || c.Rules.DirectoryModule
		r.PackageScopedBareNames = r.PackageScopedBareNames || c.Rules.PackageScopedBareNames
		r.EmptyPackageDirScoped = r.EmptyPackageDirScoped || c.Rules.EmptyPackageDirScoped
		r.NestedTypeMembers = r.NestedTypeMembers || c.Rules.NestedTypeMembers
		r.IncludeFileExportsBare = r.IncludeFileExportsBare || c.Rules.IncludeFileExportsBare
		r.DirectoryManifest = r.DirectoryManifest || c.Rules.DirectoryManifest
		r.ImportRefName = r.ImportRefName || c.Rules.ImportRefName
		r.DestExport = r.DestExport || c.Rules.DestExport
		r.RejectDunderRename = r.RejectDunderRename || c.Rules.RejectDunderRename
	}
	return r
}

// ImportLine is the host (as-import TEMPLATE) for lang, or "".
func (p *Packs) ImportLine(lang string) string {
	c := p.ImportLineInfo(lang)
	return ingestutil.EmitSeq(c.Tokens, nil)
}

// Layout is host (as-layout NAME…) for lang, or nil.
func (p *Packs) Layout(lang string) []string {
	if p == nil || p.prog == nil {
		return nil
	}
	return p.prog.layoutNames(lang)
}

// AtomicSpans is host (as-atomic TEXT) for lang.
func (p *Packs) AtomicSpans(lang string) []string {
	if p == nil || p.prog == nil {
		return nil
	}
	return p.prog.atomicTexts(lang)
}

// ImportLineInfo is the compiled as-import template and slots for lang.
func (p *Packs) ImportLineInfo(lang string) project.ImportLineClaim {
	if p == nil || p.prog == nil {
		return project.ImportLineClaim{}
	}
	return p.prog.importLineClaim(lang)
}

// PackageLineInfo is the compiled as-package token seq for lang.
func (p *Packs) PackageLineInfo(lang string) project.PackageLineClaim {
	if p == nil || p.prog == nil {
		return project.PackageLineClaim{}
	}
	return p.prog.packageLineClaim(lang)
}

// DocstringKind is the host (as-docstring KIND) for lang, or "".
func (p *Packs) DocstringKind(lang string) string {
	if p == nil || p.prog == nil {
		return ""
	}
	return p.prog.docstringKind(lang)
}

// GrammarForLanguage is the as-grammar id for lang, or lang itself.
func (p *Packs) GrammarForLanguage(lang string) string {
	if p == nil || p.prog == nil {
		return lang
	}
	return p.prog.grammarID(lang)
}

// LanguageInFamily is true when lang is a surface in family.
func (p *Packs) LanguageInFamily(lang, family string) bool {
	if p == nil {
		return false
	}
	return project.LanguageInFamily(p.Families(), lang, family)
}

// LanguagesInFamily is host langs claimed for family.
func (p *Packs) LanguagesInFamily(family string) []string {
	if p == nil {
		return nil
	}
	return project.LanguagesInFamily(p.Families(), family)
}

// FamilyForLanguage is the as-family id for lang, or "".
func (p *Packs) FamilyForLanguage(lang string) string {
	if p == nil || p.prog == nil || lang == "" {
		return ""
	}
	for _, c := range p.prog.Families {
		if c.Lang == lang {
			return c.Family
		}
	}
	return ""
}

// HostLanguage is the unique pack host language for relPath.
func (p *Packs) HostLanguage(relPath string) (lang string, ok bool, err error) {
	if p == nil {
		return "", false, nil
	}
	return hostLanguageFrom(p.prog, relPath)
}
