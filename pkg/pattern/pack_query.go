package pattern

import (
	"context"
	"fmt"
	"github.com/lewtec/patlint/pkg/project"
	"path"
	"sort"
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/store"
	"github.com/lewtec/patlint/pkg/tape"
)

var _ ingest.PackQueries = (*LispVM)(nil)

// ResolveImport is family resolve for the importer host, else ./ ../ → path:.
// No raw spec: unknown bare tokens are "".
func (vm *LispVM) ResolveImport(spec string, ctx ingest.ImportResolveContext) string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ""
	}
	if lang, ok := vm.HostLanguage(ctx.ImporterPath); ok {
		if fid := vm.FamilyForLanguage(lang); fid != "" {
			if f, ok := ingest.FamilyByID(fid); ok {
				if got := f.ResolveImport(spec, ctx); got != "" {
					return got
				}
			}
		}
	}
	if !(strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") || spec == "." || spec == "./.") {
		return ""
	}
	rel := ingest.RelativeImportPath(ctx.ImporterPath, spec)
	if rel == "" || rel == "." {
		return ingest.FileRef("./")
	}
	return ingest.FileRef("./" + rel)
}

// ImportNeedFromRef maps a product ref using this VM's as-family claims.
// Provider must be the host lang or family id. import-ref-name appends ::Name.
func (vm *LispVM) ImportNeedFromRef(lang, ref string) (ingest.ImportNeed, bool) {
	if vm == nil || lang == "" {
		return ingest.ImportNeed{}, false
	}
	ref = strings.TrimSpace(strings.TrimPrefix(ref, "@"))
	if ref == "" {
		return ingest.ImportNeed{}, false
	}
	r := ingest.ParseReference(ref)
	var fam string
	for _, c := range vm.Families() {
		if c.Lang == lang {
			fam = c.Family
			break
		}
	}
	if r.Provider != lang && (fam == "" || r.Provider != fam) {
		return ingest.ImportNeed{}, false
	}
	path := strings.TrimSpace(r.Path)
	if path == "" || path == "." || path == "./" {
		return ingest.ImportNeed{}, false
	}
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		return ingest.ImportNeed{}, false
	}
	if r.Name != "" {
		for _, c := range vm.Families() {
			if c.Family == fam && c.Rules.ImportRefName {
				path = path + "::" + r.Name
				break
			}
		}
	}
	return ingest.ImportNeed{ImportPath: path}, true
}

// RulesForLanguage is this VM's compiled as-family knobs for lang.
func (vm *LispVM) RulesForLanguage(lang string) project.LanguageRules {
	if vm == nil {
		return project.LanguageRules{}
	}
	return rulesForLang(vm.Families(), lang)
}

// FamilyForLanguage is this VM's as-family id for lang, or "".
func (vm *LispVM) FamilyForLanguage(lang string) string {
	if vm == nil || vm.p == nil {
		return ""
	}
	return vm.p.FamilyForLanguage(lang)
}

// LanguageInFamily is true when lang is a surface in family.
func (vm *LispVM) LanguageInFamily(lang, family string) bool {
	if vm == nil {
		return false
	}
	return project.LanguageInFamily(vm.Families(), lang, family)
}

// LanguagesInFamily is host langs this VM claims for family.
func (vm *LispVM) LanguagesInFamily(family string) []string {
	if vm == nil {
		return nil
	}
	return project.LanguagesInFamily(vm.Families(), family)
}

// AtomicSpans is host (as-atomic TEXT) for lang.
func (vm *LispVM) AtomicSpans(lang string) []string {
	if vm == nil || vm.p == nil {
		return nil
	}
	return vm.p.AtomicSpans(lang)
}

// tapePolicyForLang is DefaultPolicy plus this VM's as-atomic texts for lang.
func (vm *LispVM) tapePolicyForLang(lang string) tape.Policy {
	if vm == nil {
		return tape.DefaultPolicy()
	}
	return tape.WithAtomicTexts(vm.AtomicSpans(lang))
}

// TapePolicy is the pack tape policy for rel's host language (as-atomic).
func (vm *LispVM) TapePolicy(rel string) tape.Policy {
	if vm == nil {
		return tape.DefaultPolicy()
	}
	lang, ok, err := vm.AttributeHost(rel)
	if err != nil || !ok {
		return tape.DefaultPolicy()
	}
	return vm.tapePolicyForLang(lang)
}

// ImportLine is the first-import template for lang ($path = spec).
func (vm *LispVM) ImportLine(lang string) string {
	c := vm.ImportLineInfo(lang)
	return ingestutil.EmitSeq(c.Tokens, nil)
}

// Layout is host (as-layout NAME…) for lang, or nil.
func (vm *LispVM) Layout(lang string) []string {
	if vm == nil || vm.p == nil {
		return nil
	}
	return vm.p.Layout(lang)
}

// ImportLineInfo is the compiled as-import template and slots for lang.
func (vm *LispVM) ImportLineInfo(lang string) project.ImportLineClaim {
	if vm == nil || vm.p == nil {
		return project.ImportLineClaim{}
	}
	return vm.p.ImportLineInfo(lang)
}

// PackageLineInfo is the compiled as-package token seq for lang.
func (vm *LispVM) PackageLineInfo(lang string) project.PackageLineClaim {
	if vm == nil || vm.p == nil {
		return project.PackageLineClaim{}
	}
	return vm.p.PackageLineInfo(lang)
}

// ScopeNodeTypes is pack as-scope (node T) for lang (docs + abstract units).
func (vm *LispVM) ScopeNodeTypes(lang string) []string {
	if vm == nil || vm.p == nil || vm.p.Program() == nil {
		return nil
	}
	return vm.p.Program().ScopeNodeTypes(lang)
}

// DocstringKind is pack (as-docstring KIND) for lang, or "".
func (vm *LispVM) DocstringKind(lang string) string {
	if vm == nil || vm.p == nil {
		return ""
	}
	return vm.p.DocstringKind(lang)
}

// ClassifyLeaf is pack paint type/token → highlight class for lang.
func (vm *LispVM) ClassifyLeaf(lang, grammarType string) string {
	if vm == nil || vm.p == nil || vm.p.Program() == nil {
		return ""
	}
	return vm.p.Program().ClassifyLeaf(lang, grammarType)
}

// GrammarForLanguage is the parse grammar id for a pack language (as-grammar or lang).
func (vm *LispVM) GrammarForLanguage(lang string) string {
	if vm == nil || vm.p == nil {
		return lang
	}
	return vm.p.GrammarForLanguage(lang)
}

// DirectoryRepresentants is the pack as-directory-representant path globs.
func (vm *LispVM) DirectoryRepresentants() []string {
	if vm == nil || vm.p == nil {
		return nil
	}
	return vm.p.DirectoryRepresentants()
}

// MatchDirectoryRepresentant uses the same path glob interpreter as as-language.
func (vm *LispVM) MatchDirectoryRepresentant(rel string) bool {
	if vm == nil {
		return false
	}
	rel = strings.TrimPrefix(filepathToSlash(rel), "./")
	if rel == "" {
		return false
	}
	for _, g := range vm.DirectoryRepresentants() {
		if MatchPathGlob(g, rel) {
			return true
		}
	}
	return false
}

// Families is the compiled pack as-family claims.
func (vm *LispVM) Families() []project.FamilyClaim {
	if vm == nil || vm.p == nil {
		return nil
	}
	return vm.p.Families()
}

// AttributeHost is HostLanguage with a trailing error (ingest.PackQueries).
func (vm *LispVM) AttributeHost(relPath string) (lang string, ok bool, err error) {
	lang, ok = vm.HostLanguage(relPath)
	return lang, ok, nil
}

// Extract runs pack extract on an already-parsed host tree.
func (vm *LispVM) Extract(ctx context.Context, sess *project.Session, lang string, root *sitter.Node, source []byte, relPath string) (*project.FileExtract, error) {
	_ = lang
	if vm == nil {
		return nil, fmt.Errorf("%w: lispvm: nil", ErrExtract)
	}
	if vm.p == nil || vm.p.Program() == nil {
		return nil, fmt.Errorf("%w: lispvm: no extract program", ErrExtract)
	}
	return vm.p.Program().ExtractErr(ctx, sess, root, source, relPath)
}

// ExtractInto writes ingest tuples for one parsed file into st.
func (vm *LispVM) ExtractInto(ctx context.Context, sess *project.Session, st *store.Store, lang string, root *sitter.Node, source []byte, relPath string) error {
	_ = lang
	if vm == nil {
		return fmt.Errorf("%w: lispvm: nil", ErrExtract)
	}
	if vm.p == nil || vm.p.Program() == nil {
		return fmt.Errorf("%w: lispvm: no extract program", ErrExtract)
	}
	return vm.p.Program().ExtractInto(ctx, sess, st, root, source, relPath)
}

// PathHasEmbeds is true when a pack embed action claims relPath.
func (vm *LispVM) PathHasEmbeds(relPath string) bool {
	if vm == nil || vm.p == nil || vm.p.Program() == nil {
		return false
	}
	rel := stringsTrimDotSlash(filepathToSlash(relPath))
	for _, act := range vm.p.Program().Actions {
		if act.Embed && actionAcceptsPath(act.Paths, rel) {
			return true
		}
	}
	if base := path.Base(rel); base != rel {
		for _, act := range vm.p.Program().Actions {
			if act.Embed && actionAcceptsPath(act.Paths, base) {
				return true
			}
		}
	}
	return false
}

// EmbedRegions lists nested as-language spans on the host tree.
func (vm *LispVM) EmbedRegions(ctx context.Context, relPath string, source []byte, hostRoot *sitter.Node) []project.EmbeddedSource {
	if vm == nil || vm.p == nil || vm.p.Program() == nil {
		return nil
	}
	rel := stringsTrimDotSlash(filepathToSlash(relPath))
	loci := uniqueEmbedRegions(ctx, nil, vm.p.Program(), rel, relPath, source, hostRoot, nil)
	out := make([]project.EmbeddedSource, 0, len(loci))
	for _, e := range loci {
		out = append(out, project.EmbeddedSource{
			Offset:   e.base,
			Source:   e.content,
			Language: e.lang,
			FileHint: embedFileHint(e.lang, vm.PathExtsForLanguage(e.lang)...),
		})
		e.close()
	}
	return out
}

// PathExtsForLanguage is literal extensions from pack path globs for lang.
func (vm *LispVM) PathExtsForLanguage(lang string) []string {
	if vm == nil || vm.p == nil || vm.p.Program() == nil || lang == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, act := range vm.p.Program().Actions {
		if act.Embed || act.HostLang != lang {
			continue
		}
		for _, g := range act.Paths {
			i := strings.LastIndex(g, ".")
			if i < 0 || i >= len(g)-1 {
				continue
			}
			ext := g[i:]
			if strings.ContainsAny(ext, "/*?") {
				continue
			}
			if _, ok := seen[ext]; ok {
				continue
			}
			seen[ext] = struct{}{}
			out = append(out, ext)
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}
