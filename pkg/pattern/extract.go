package pattern

import (
	"context"
	"fmt"
	"github.com/lewtec/patlint/pkg/project"
	"sort"
	"strings"

	"github.com/lewtec/patlint/pkg/datalog"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/store"
	"github.com/lewtec/patlint/pkg/tape"
)

// ExtractKind is a build handler on matcher sites (SPEC.md Extract dialect).
type ExtractKind int

const (
	ExtractPackage ExtractKind = iota
	ExtractAtom
	ExtractUse
	ExtractImport
	// ExtractReexport records a barrel/forward hop (export … from / re-export).
	ExtractReexport
	// ExtractDefault sets FileExtract.DefaultExport (module primary export).
	ExtractDefault
	// ExtractPaint marks a highlight class on matched loci (tok-kw, tok-str, …).
	ExtractPaint
	// ExtractScope records a span; engine nests by containment after extract.
	ExtractScope
	// ExtractFlow records a control-flow increment (structural / hybrid).
	ExtractFlow
)

// ExtractAction is one build form: match sites, then write FileExtract rows
// and/or paint TokenClass on spans.
type ExtractAction struct {
	Kind ExtractKind
	// Paths are globs from enclosing (under (path G…) …); empty = always (global paint).
	Paths []string
	// HostLang is the file host language (path-level as-language).
	HostLang string
	// Lang is the region language for matching (host or embed).
	Lang string
	// Embed is true when Lang is a nested as-language under a place (reparse).
	Embed bool
	// Region finds host spans to reparse when Embed (nil for host actions).
	Region *CompiledMatcher
	// Matcher finds sites on the host tree (host) or embed tree (embed).
	Matcher *CompiledMatcher
	// Take is an optional capture name for the locus (same meaning as rewrite take).
	Take string
	// ScopeField is the capture/field name for UsageDef.Scope (from (scope NAME) on as-use).
	// When ScopeNode is also set, Scope is read from the enclosing AST node of that type
	// (outermost (node T) in the place nest), avoiding clash with body capture "name".
	ScopeField string
	// ScopeNode is a tree-sitter node type to walk for ScopeField (e.g. function_declaration).
	ScopeNode string
	// TokenClass is set for ExtractPaint (ingest.HL* / tok-*).
	TokenClass string
	// HoleOnly is as-decl: span is a move hole, not a resolve scope.
	HoleOnly bool
	// FlowClass is as-flow structural|hybrid.
	FlowClass string
	// Exported is as-atom public|private. Unused on other kinds.
	Exported bool
	// NodeType is the innermost (node T) of the place nest (as-scope / as-atom).
	NodeType string
	// PaintLeaves are token/node type names from as-keyword / as-ident / … matchers.
	PaintLeaves []string
}

// ExtractProgram is a compiled language extract pack (same sexpr dialect as scripts).
type ExtractProgram struct {
	Path string
	// Language is the primary language id (first as-language in the pack).
	Language string
	Actions  []ExtractAction
	// AllowList is leftover. as-atom public|private writes AtomDef.Exported.
	AllowList func(name string) bool
	// Families are host as-family claims (lang → family id).
	Families []project.FamilyClaim
	// Grammars are host as-grammar claims (lang → sitter grammar id).
	Grammars []project.GrammarClaim
	// ImportLines are host (as-import (seq …)) token forms.
	ImportLines []project.ImportLineClaim
	// PackageLines are host (as-package (seq …)) token forms.
	PackageLines []project.PackageLineClaim
	// AtomicSpans are host (as-atomic TEXT) forms (lang → exact tape cell texts).
	AtomicSpans []project.AtomicSpanClaim
	// Docstrings are host (as-docstring KIND) forms.
	Docstrings []project.DocstringClaim
	// DirectoryRepresentants are path globs from (under (path …) (as-directory-representant)).
	DirectoryRepresentants []string
	// Layouts are host (as-layout NAME…) forms (lang → section order).
	Layouts []project.LayoutClaim
}

// extractScope is path + languages + place nest while loading pack forms.
// nest holds matcher place forms (e.g. (node "function_declaration")) that
// wrap handlers: (under place … (as-atom public …)).
// When embed is set, nest is relative to the embed tree; embedRegion is the
// host place that is reparsed (SPEC.md Regions).
type extractScope struct {
	paths       []string
	hostLang    string // path-level as-language
	lang        string // current region language
	embed       bool   // lang is embed (nested as-language under a place)
	embedRegion []any  // host place nest for reparse
	nest        []any  // outer → inner place forms (host or embed body)
}

func (sc extractScope) hasPath() bool { return len(sc.paths) > 0 }

func (sc extractScope) cloneNest() []any {
	if len(sc.nest) == 0 {
		return nil
	}
	out := make([]any, len(sc.nest))
	copy(out, sc.nest)
	return out
}

func (sc extractScope) cloneEmbedRegion() []any {
	if len(sc.embedRegion) == 0 {
		return nil
	}
	out := make([]any, len(sc.embedRegion))
	copy(out, sc.embedRegion)
	return out
}

// applyAsLanguage sets host or embed language on scope (SPEC.md Regions).
// Host as-language under path always records a path claim action so
// AttributeHost works even when the pack has no as-atom/paint handlers
// (e.g. language_rft.rft is only path + as-language).
func applyAsLanguage(sc extractScope, prog *ExtractProgram, id string) (extractScope, error) {
	if id == "" {
		return sc, fmt.Errorf("%w: as-language id empty", ErrExtract)
	}
	if len(sc.nest) == 0 && !sc.embed {
		// Host language for this path scope.
		sc.hostLang = id
		sc.lang = id
		sc.embed = false
		sc.embedRegion = nil
		if prog.Language == "" {
			prog.Language = id
		}
		if sc.hasPath() {
			// Matcher nil → Extract skips; AttributeHost reads Paths+HostLang.
			prog.Actions = append(prog.Actions, ExtractAction{
				Paths:    append([]string(nil), sc.paths...),
				HostLang: id,
				Lang:     id,
			})
		}
		return sc, nil
	}
	// Nested under a place → embed region: reparse nest as id.
	if len(sc.nest) == 0 {
		return sc, fmt.Errorf("%w: embed as-language %q needs a place (under …)", ErrExtract, id)
	}
	sc.embedRegion = sc.cloneNest()
	sc.nest = nil // body places are relative to the embed tree
	sc.lang = id
	sc.embed = true
	// Record a region claim even when the embed has no as-* body verbs so
	// paint/tape can discover reparse sites (SPEC.md Regions: nothing skips embeds).
	if prog != nil && sc.hasPath() {
		regForm, err := wrapPlaceNest(sc.embedRegion, nil)
		if err == nil {
			if regAct, err := compileExtractBody(regForm); err == nil && regAct.Matcher != nil {
				prog.Actions = append(prog.Actions, ExtractAction{
					Paths:    append([]string(nil), sc.paths...),
					HostLang: sc.hostLang,
					Lang:     id,
					Embed:    true,
					Region:   regAct.Matcher,
				})
			}
		}
	}
	return sc, nil
}

// applyAsFamily records a host lang→family claim. Host as-language must come first.
func applyAsFamily(sc extractScope, prog *ExtractProgram, list []any, path string, i int) error {
	if !sc.hasPath() {
		return fmt.Errorf("%w: %s: form %d: as-family must be inside (under (path …) …)", ErrExtract, path, i)
	}
	if sc.hostLang == "" {
		return fmt.Errorf("%w: %s: form %d: as-family needs host (as-language ID) first", ErrExtract, path, i)
	}
	if sc.embed {
		return fmt.Errorf("%w: %s: form %d: as-family is host-only (not under embed as-language)", ErrExtract, path, i)
	}
	if len(list) < 2 {
		return fmt.Errorf("%w: %s: form %d: as-family wants id (string)", ErrExtract, path, i)
	}
	id, err := extractAsString(list[1])
	if err != nil {
		return fmt.Errorf("%w: %s: form %d: as-family id (string): %v", ErrExtract, path, i, err)
	}
	knobs, err := parseFamilyKnobs(list[2:], path, i)
	if err != nil {
		return err
	}
	if prog == nil {
		return nil
	}
	for fi := range prog.Families {
		c := &prog.Families[fi]
		if c.Lang == sc.hostLang {
			if c.Family != id {
				return fmt.Errorf("%w: %s: form %d: language %q already in family %q", ErrExtract, path, i, sc.hostLang, c.Family)
			}
			c.Rules = orLanguageRules(c.Rules, knobs)
			return nil
		}
	}
	prog.Families = append(prog.Families, project.FamilyClaim{Lang: sc.hostLang, Family: id, Rules: knobs})
	return nil
}

// applyAsLayout records a host (as-layout NAME…) section order.
func applyAsLayout(sc extractScope, prog *ExtractProgram, list []any, path string, i int) error {
	if !sc.hasPath() {
		return fmt.Errorf("%w: %s: form %d: as-layout must be inside (under (path …) …)", ErrExtract, path, i)
	}
	if sc.hostLang == "" {
		return fmt.Errorf("%w: %s: form %d: as-layout needs host (as-language ID) first", ErrExtract, path, i)
	}
	if sc.embed || len(sc.nest) > 0 {
		return fmt.Errorf("%w: %s: form %d: as-layout is host-only", ErrExtract, path, i)
	}
	if len(list) < 2 {
		return fmt.Errorf("%w: %s: form %d: as-layout wants names", ErrExtract, path, i)
	}
	names, err := parseLayoutNames(list[1:], path, i)
	if err != nil {
		return err
	}
	if prog == nil {
		return nil
	}
	for _, c := range prog.Layouts {
		if c.Lang == sc.hostLang {
			if !layoutNamesEq(c.Names, names) {
				return fmt.Errorf("%w: %s: form %d: language %q already has as-layout", ErrExtract, path, i, sc.hostLang)
			}
			return nil
		}
	}
	prog.Layouts = append(prog.Layouts, project.LayoutClaim{Lang: sc.hostLang, Names: names})
	return nil
}

func parseLayoutNames(args []any, path string, form int) ([]string, error) {
	var names []string
	seen := map[string]bool{}
	for _, v := range args {
		s, err := extractAsString(v)
		if err != nil || s == "" {
			return nil, fmt.Errorf("%w: %s: form %d: as-layout name wants symbol", ErrExtract, path, form)
		}
		if seen[s] {
			return nil, fmt.Errorf("%w: %s: form %d: duplicate as-layout name %q", ErrExtract, path, form, s)
		}
		seen[s] = true
		names = append(names, s)
	}
	return names, nil
}

func layoutNamesEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (p *ExtractProgram) layoutNames(lang string) []string {
	if p == nil || lang == "" {
		return nil
	}
	for _, c := range p.Layouts {
		if c.Lang == lang {
			return append([]string(nil), c.Names...)
		}
	}
	return nil
}

// applyAsDirectoryRepresentant records enclosing path globs as dir-entry files.
func applyAsDirectoryRepresentant(sc extractScope, prog *ExtractProgram, list []any, path string, i int) error {
	if !sc.hasPath() {
		return fmt.Errorf("%w: %s: form %d: as-directory-representant must be inside (under (path …) …)", ErrExtract, path, i)
	}
	if sc.embed {
		return fmt.Errorf("%w: %s: form %d: as-directory-representant is host-only", ErrExtract, path, i)
	}
	if len(sc.nest) > 0 {
		return fmt.Errorf("%w: %s: form %d: as-directory-representant wants a path place, not a matcher nest", ErrExtract, path, i)
	}
	if len(list) != 1 {
		return fmt.Errorf("%w: %s: form %d: as-directory-representant takes no arguments", ErrExtract, path, i)
	}
	if prog == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, g := range prog.DirectoryRepresentants {
		seen[g] = true
	}
	for _, g := range sc.paths {
		if g == "" || seen[g] {
			continue
		}
		seen[g] = true
		prog.DirectoryRepresentants = append(prog.DirectoryRepresentants, g)
	}
	return nil
}

func parseFamilyKnobs(args []any, path string, form int) (project.LanguageRules, error) {
	var r project.LanguageRules
	for _, v := range args {
		s, err := extractAsString(v)
		if err != nil {
			return r, fmt.Errorf("%w: %s: form %d: as-family knob wants symbol", ErrExtract, path, form)
		}
		switch s {
		case "directory-module":
			r.DirectoryModule = true
		case "package-scoped-bare-names":
			r.PackageScopedBareNames = true
		case "empty-package-dir-scoped":
			r.EmptyPackageDirScoped = true
		case "nested-type-members":
			r.NestedTypeMembers = true
		case "include-file-exports-bare":
			r.IncludeFileExportsBare = true
		case "directory-manifest":
			r.DirectoryManifest = true
		case "import-ref-name":
			r.ImportRefName = true
		case "dest-export":
			r.DestExport = true
		case "reject-dunder-rename":
			r.RejectDunderRename = true
		default:
			return r, fmt.Errorf("%w: %s: form %d: unknown as-family knob %q", ErrExtract, path, form, s)
		}
	}
	return r, nil
}

func orLanguageRules(a, b project.LanguageRules) project.LanguageRules {
	return project.LanguageRules{
		DirectoryModule:        a.DirectoryModule || b.DirectoryModule,
		PackageScopedBareNames: a.PackageScopedBareNames || b.PackageScopedBareNames,
		EmptyPackageDirScoped:  a.EmptyPackageDirScoped || b.EmptyPackageDirScoped,
		NestedTypeMembers:      a.NestedTypeMembers || b.NestedTypeMembers,
		IncludeFileExportsBare: a.IncludeFileExportsBare || b.IncludeFileExportsBare,
		DirectoryManifest:      a.DirectoryManifest || b.DirectoryManifest,
		ImportRefName:          a.ImportRefName || b.ImportRefName,
		DestExport:             a.DestExport || b.DestExport,
		RejectDunderRename:     a.RejectDunderRename || b.RejectDunderRename,
	}
}

// applyAsGrammar records a host lang→grammar claim. Host as-language must come first.
func applyAsGrammar(sc extractScope, prog *ExtractProgram, list []any, path string, i int) error {
	if !sc.hasPath() {
		return fmt.Errorf("%w: %s: form %d: as-grammar must be inside (under (path …) …)", ErrExtract, path, i)
	}
	if sc.hostLang == "" {
		return fmt.Errorf("%w: %s: form %d: as-grammar needs host (as-language ID) first", ErrExtract, path, i)
	}
	if sc.embed {
		return fmt.Errorf("%w: %s: form %d: as-grammar is host-only (not under embed as-language)", ErrExtract, path, i)
	}
	if len(list) < 2 {
		return fmt.Errorf("%w: %s: form %d: as-grammar wants id (string)", ErrExtract, path, i)
	}
	id, err := extractAsString(list[1])
	if err != nil {
		return fmt.Errorf("%w: %s: form %d: as-grammar id (string): %v", ErrExtract, path, i, err)
	}
	if prog == nil {
		return nil
	}
	for _, c := range prog.Grammars {
		if c.Lang == sc.hostLang {
			if c.Grammar != id {
				return fmt.Errorf("%w: %s: form %d: language %q already uses grammar %q", ErrExtract, path, i, sc.hostLang, c.Grammar)
			}
			return nil
		}
	}
	prog.Grammars = append(prog.Grammars, project.GrammarClaim{Lang: sc.hostLang, Grammar: id})
	return nil
}

func hostTokenSeq(sc extractScope, list []any) bool {
	if !sc.hasPath() || sc.hostLang == "" || sc.embed || len(sc.nest) > 0 || len(list) < 2 {
		return false
	}
	if _, err := extractAsString(list[1]); err == nil {
		return true
	}
	seq, ok := list[1].([]any)
	return ok && len(seq) > 0 && seq[0] == "seq"
}

// applyAsImportSeq records a host (as-import (seq TOKEN…)). path/leaf/qual are fields.
func applyAsImportSeq(sc extractScope, prog *ExtractProgram, list []any, path string, i int) error {
	if !sc.hasPath() {
		return fmt.Errorf("%w: %s: form %d: as-import seq must be inside (under (path …) …)", ErrExtract, path, i)
	}
	if sc.hostLang == "" {
		return fmt.Errorf("%w: %s: form %d: as-import seq needs host (as-language ID) first", ErrExtract, path, i)
	}
	if sc.embed || len(sc.nest) > 0 {
		return fmt.Errorf("%w: %s: form %d: as-import seq is host-only", ErrExtract, path, i)
	}
	if len(list) < 2 {
		return fmt.Errorf("%w: %s: form %d: as-import wants (seq …)", ErrExtract, path, i)
	}
	if _, err := extractAsString(list[1]); err == nil {
		return fmt.Errorf("%w: %s: form %d: as-import uses (seq tokens), not a string", ErrExtract, path, i)
	}
	seq, ok := list[1].([]any)
	if !ok || len(seq) < 2 || seq[0] != "seq" {
		return fmt.Errorf("%w: %s: form %d: as-import wants (seq TOKEN…)", ErrExtract, path, i)
	}
	toks, err := parseImportSeq(seq[1:])
	if err != nil {
		return fmt.Errorf("%w: %s: form %d: %v", ErrExtract, path, i, err)
	}
	if !seqHasField(toks, "path") {
		return fmt.Errorf("%w: %s: form %d: as-import seq needs path", ErrExtract, path, i)
	}
	if prog == nil {
		return nil
	}
	claim := project.ImportLineClaim{
		Lang:       sc.hostLang,
		Tokens:     toks,
		HasLeaf:    seqHasField(toks, "leaf"),
		HasQual:    seqHasField(toks, "qual"),
		QuotedPath: seqQuotedPath(toks),
	}
	for _, c := range prog.ImportLines {
		if c.Lang == sc.hostLang {
			if strings.Join(c.Tokens, "\x00") != strings.Join(toks, "\x00") {
				return fmt.Errorf("%w: %s: form %d: language %q already has as-import seq", ErrExtract, path, i, sc.hostLang)
			}
			return nil
		}
	}
	prog.ImportLines = append(prog.ImportLines, claim)
	return nil
}

func parseImportSeq(parts []any) ([]string, error) {
	return parseHostSeq(parts, "as-import")
}

func applyAsPackageSeq(sc extractScope, prog *ExtractProgram, list []any, path string, i int) error {
	if !sc.hasPath() {
		return fmt.Errorf("%w: %s: form %d: as-package seq must be inside (under (path …) …)", ErrExtract, path, i)
	}
	if sc.hostLang == "" {
		return fmt.Errorf("%w: %s: form %d: as-package seq needs host (as-language ID) first", ErrExtract, path, i)
	}
	if sc.embed || len(sc.nest) > 0 {
		return fmt.Errorf("%w: %s: form %d: as-package seq is host-only", ErrExtract, path, i)
	}
	if len(list) < 2 {
		return fmt.Errorf("%w: %s: form %d: as-package wants (seq …)", ErrExtract, path, i)
	}
	if _, err := extractAsString(list[1]); err == nil {
		return fmt.Errorf("%w: %s: form %d: as-package uses (seq tokens), not a string", ErrExtract, path, i)
	}
	seq, ok := list[1].([]any)
	if !ok || len(seq) < 2 || seq[0] != "seq" {
		return fmt.Errorf("%w: %s: form %d: as-package wants (seq TOKEN…)", ErrExtract, path, i)
	}
	toks, err := parseHostSeq(seq[1:], "as-package")
	if err != nil {
		return fmt.Errorf("%w: %s: form %d: %v", ErrExtract, path, i, err)
	}
	if !seqHasField(toks, "pkg") {
		return fmt.Errorf("%w: %s: form %d: as-package seq needs pkg", ErrExtract, path, i)
	}
	if prog == nil {
		return nil
	}
	claim := project.PackageLineClaim{Lang: sc.hostLang, Tokens: toks}
	for _, c := range prog.PackageLines {
		if c.Lang == sc.hostLang {
			if strings.Join(c.Tokens, "\x00") != strings.Join(toks, "\x00") {
				return fmt.Errorf("%w: %s: form %d: language %q already has as-package seq", ErrExtract, path, i, sc.hostLang)
			}
			return nil
		}
	}
	prog.PackageLines = append(prog.PackageLines, claim)
	return nil
}

func parseHostSeq(parts []any, verb string) ([]string, error) {
	var toks []string
	for _, p := range parts {
		s, err := extractAsString(p)
		if err != nil {
			return nil, fmt.Errorf("%s seq token wants string or field", verb)
		}
		toks = append(toks, s)
	}
	return toks, nil
}

func seqHasField(toks []string, name string) bool {
	for _, t := range toks {
		if t == name {
			return true
		}
	}
	return false
}

func seqQuotedPath(toks []string) bool {
	for i, t := range toks {
		if t != "path" || i == 0 {
			continue
		}
		prev := toks[i-1]
		if prev == `"` || prev == "'" {
			return true
		}
	}
	return false
}

func (p *ExtractProgram) importLineClaim(lang string) project.ImportLineClaim {
	if p == nil || lang == "" {
		return project.ImportLineClaim{}
	}
	for _, c := range p.ImportLines {
		if c.Lang == lang {
			return c
		}
	}
	return project.ImportLineClaim{}
}

func (p *ExtractProgram) packageLineClaim(lang string) project.PackageLineClaim {
	if p == nil || lang == "" {
		return project.PackageLineClaim{}
	}
	for _, c := range p.PackageLines {
		if c.Lang == lang {
			return c
		}
	}
	return project.PackageLineClaim{}
}

func (p *ExtractProgram) docstringKind(lang string) string {
	if p == nil || lang == "" {
		return ""
	}
	for _, c := range p.Docstrings {
		if c.Lang == lang {
			return c.Kind
		}
	}
	return ""
}

// applyAsAtomic records a host (as-atomic TEXT) tape cell.
func applyAsAtomic(sc extractScope, prog *ExtractProgram, list []any, path string, i int) error {
	if !sc.hasPath() {
		return fmt.Errorf("%w: %s: form %d: as-atomic must be inside (under (path …) …)", ErrExtract, path, i)
	}
	if sc.hostLang == "" {
		return fmt.Errorf("%w: %s: form %d: as-atomic needs host (as-language ID) first", ErrExtract, path, i)
	}
	if sc.embed || len(sc.nest) > 0 {
		return fmt.Errorf("%w: %s: form %d: as-atomic is host-only", ErrExtract, path, i)
	}
	if len(list) != 2 {
		return fmt.Errorf("%w: %s: form %d: as-atomic wants one string", ErrExtract, path, i)
	}
	text, err := extractAsString(list[1])
	if err != nil {
		return fmt.Errorf("%w: %s: form %d: as-atomic text: %v", ErrExtract, path, i, err)
	}
	if text == "" {
		return fmt.Errorf("%w: %s: form %d: as-atomic text is empty", ErrExtract, path, i)
	}
	if prog == nil {
		return nil
	}
	for _, c := range prog.AtomicSpans {
		if c.Lang == sc.hostLang && c.Text == text {
			return nil
		}
	}
	prog.AtomicSpans = append(prog.AtomicSpans, project.AtomicSpanClaim{Lang: sc.hostLang, Text: text})
	return nil
}

func (p *ExtractProgram) atomicTexts(lang string) []string {
	if p == nil || lang == "" {
		return nil
	}
	var out []string
	for _, c := range p.AtomicSpans {
		if c.Lang == lang && c.Text != "" {
			out = append(out, c.Text)
		}
	}
	return out
}

func (p *ExtractProgram) hostLangFor(rel string) string {
	lang := ""
	if p != nil {
		lang = p.Language
	}
	if lang == "" {
		lang = "unknown"
	}
	if p == nil {
		return lang
	}
	for _, act := range p.Actions {
		if actionAcceptsPath(act.Paths, rel) && act.HostLang != "" {
			return act.HostLang
		}
	}
	return lang
}

func (p *ExtractProgram) tapePolicy(lang string) tape.Policy {
	if p == nil {
		return tape.DefaultPolicy()
	}
	return tape.WithAtomicTexts(p.atomicTexts(lang))
}

// applyAsDocstring records a host (as-docstring KIND).
func applyAsDocstring(sc extractScope, prog *ExtractProgram, list []any, path string, i int) error {
	if !sc.hasPath() {
		return fmt.Errorf("%w: %s: form %d: as-docstring must be inside (under (path …) …)", ErrExtract, path, i)
	}
	if sc.hostLang == "" {
		return fmt.Errorf("%w: %s: form %d: as-docstring needs host (as-language ID) first", ErrExtract, path, i)
	}
	if sc.embed || len(sc.nest) > 0 {
		return fmt.Errorf("%w: %s: form %d: as-docstring is host-only", ErrExtract, path, i)
	}
	if len(list) != 2 {
		return fmt.Errorf("%w: %s: form %d: as-docstring wants one kind", ErrExtract, path, i)
	}
	kind, err := extractAsString(list[1])
	if err != nil {
		return fmt.Errorf("%w: %s: form %d: as-docstring kind: %v", ErrExtract, path, i, err)
	}
	switch kind {
	case ingest.DocstringCommentBefore, ingest.DocstringCommentBeforeC, ingest.DocstringBodyString:
	default:
		return fmt.Errorf("%w: %s: form %d: unknown as-docstring kind %q", ErrExtract, path, i, kind)
	}
	if prog == nil {
		return nil
	}
	for _, c := range prog.Docstrings {
		if c.Lang == sc.hostLang {
			if c.Kind != kind {
				return fmt.Errorf("%w: %s: form %d: language %q already has as-docstring %q", ErrExtract, path, i, sc.hostLang, c.Kind)
			}
			return nil
		}
	}
	prog.Docstrings = append(prog.Docstrings, project.DocstringClaim{Lang: sc.hostLang, Kind: kind})
	return nil
}

func (p *ExtractProgram) grammarID(lang string) string {
	if p == nil || lang == "" {
		return lang
	}
	for _, c := range p.Grammars {
		if c.Lang == lang && c.Grammar != "" {
			return c.Grammar
		}
	}
	return lang
}

// ScopeNodeTypes is unique sorted (node T) from as-scope for lang (not as-decl).
func (p *ExtractProgram) ScopeNodeTypes(lang string) []string {
	if p == nil || lang == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, act := range p.Actions {
		if act.Kind != ExtractScope || act.HoleOnly || act.Lang != lang || act.NodeType == "" {
			continue
		}
		if _, ok := seen[act.NodeType]; ok {
			continue
		}
		seen[act.NodeType] = struct{}{}
		out = append(out, act.NodeType)
	}
	sort.Strings(out)
	return out
}

// ClassifyLeaf is pack as-keyword/as-ident/… token or node type → highlight class.
// Paint with empty Lang (highlight.rft) applies to every host; host-lang paint wins.
func (p *ExtractProgram) ClassifyLeaf(lang, grammarType string) string {
	if p == nil || grammarType == "" {
		return ""
	}
	global := ""
	for _, act := range p.Actions {
		if act.Kind != ExtractPaint || act.TokenClass == "" {
			continue
		}
		if act.Lang != "" && act.Lang != lang {
			continue
		}
		hit := false
		for _, leaf := range act.PaintLeaves {
			if leaf == grammarType {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		if act.Lang != "" && act.Lang == lang {
			return act.TokenClass
		}
		if act.Lang == "" {
			global = act.TokenClass
		}
	}
	return global
}

func paintLeafNames(v any) []string {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	h, _ := list[0].(string)
	if (h == "token" || h == "node") && len(list) >= 2 {
		if s, err := extractAsString(list[1]); err == nil && s != "" {
			return []string{s}
		}
	}
	var out []string
	for _, a := range list[1:] {
		out = append(out, paintLeafNames(a)...)
	}
	return out
}

// wrapPlaceNest builds (under p0 (under p1 … body)).
// If body is nil, the innermost place is the matcher (locus = that place).
func wrapPlaceNest(nest []any, body any) (any, error) {
	if body == nil {
		if len(nest) == 0 {
			return nil, fmt.Errorf("%w: handler needs a place (under …) or a MATCHER argument", ErrExtract)
		}
		body = nest[len(nest)-1]
		nest = nest[:len(nest)-1]
	}
	for i := len(nest) - 1; i >= 0; i-- {
		body = []any{"under", nest[i], body}
	}
	return body, nil
}

func actionAcceptsPath(paths []string, rel string) bool {
	if len(paths) == 0 {
		return true
	}
	for _, g := range paths {
		if matchPathGlob(g, rel) {
			return true
		}
	}
	return false
}

// parsePathGlobs reads one or more string globs after a path head.
// (path GLOB body…) or (path G1 G2 … body…)
func parsePathGlobs(args []any) (globs []string, body []any, err error) {
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("%w: path wants at least one glob", ErrExtract)
	}
	i := 0
	for i < len(args) {
		s, ok := args[i].(string)
		if !ok {
			break
		}
		if s == "" {
			return nil, nil, fmt.Errorf("%w: path glob empty", ErrExtract)
		}
		globs = append(globs, s)
		i++
	}
	if len(globs) == 0 {
		return nil, nil, fmt.Errorf("%w: path wants glob string(s), got %T", ErrExtract, args[0])
	}
	return globs, args[i:], nil
}

// LoadExtractPack compiles pack source (same sexp as .rft scripts).
// as-language must sit under (under (path …) …), not at the root.
// LoadExtractPack compiles pack source. Expand env is whatever SetExpandPrelude installed.
func LoadExtractPack(path, src string) (*ExtractProgram, error) {
	expandPreludeMu.RLock()
	base := expandPrelude.clone()
	expandPreludeMu.RUnlock()
	return loadExtractPack(path, src, base)
}

// loadExtractPack compiles with an explicit expand env (no Expand → prelude re-entry).
func loadExtractPack(path, src string, base expandEnv) (*ExtractProgram, error) {
	forms, err := ParseSexpDataFile(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	env := base.clone()
	if env == nil {
		env = expandEnv{}
	}
	prog := &ExtractProgram{Path: path}
	sc := extractScope{}
	for i, form := range forms {
		var err error
		sc, err = loadExtractTop(prog, env, form, sc, path, i)
		if err != nil {
			return nil, err
		}
	}
	// Defs-only files (e.g. core.rft) are valid: Actions empty, Language empty.
	// Grammar ids are checked against the Session engine at NewWalker.
	return prog, nil
}

// ValidateGrammars fails if any as-language id is unknown to eng.
func (p *ExtractProgram) ValidateGrammars(ctx context.Context, eng sitter.Engine) error {
	if p == nil {
		return nil
	}
	return validateExtractPackGrammars(ctx, eng, "pack", p)
}

func validateExtractPackGrammars(ctx context.Context, eng sitter.Engine, path string, prog *ExtractProgram) error {
	if eng == nil {
		return sitter.ErrNilEngine
	}
	seen := map[string]struct{}{}
	check := func(id string) error {
		if id == "" {
			return nil
		}
		gid := prog.grammarID(id)
		if _, ok := seen[gid]; ok {
			return nil
		}
		seen[gid] = struct{}{}
		if !eng.Has(ctx, gid) {
			return fmt.Errorf("%w: %s: unknown language %q (grammar %q not registered; use as-grammar or blank-import)", ErrExtract, path, id, gid)
		}
		return nil
	}
	if err := check(prog.Language); err != nil {
		return err
	}
	for _, act := range prog.Actions {
		if err := check(act.HostLang); err != nil {
			return err
		}
		if err := check(act.Lang); err != nil {
			return err
		}
	}
	return nil
}

func loadExtractTop(prog *ExtractProgram, env expandEnv, form any, sc extractScope, path string, i int) (extractScope, error) {
	list, ok := form.([]any)
	if !ok || len(list) == 0 {
		return sc, fmt.Errorf("%w: %s: form %d: want list", ErrExtract, path, i)
	}
	head, ok := list[0].(string)
	if !ok {
		return sc, fmt.Errorf("%w: %s: form %d: head must be symbol", ErrExtract, path, i)
	}
	switch head {
	case "def":
		name, fn, err := ParseDef(list[1:])
		if err != nil {
			return sc, fmt.Errorf("%s: form %d: %w", path, i, err)
		}
		env.Set(name, fn)
		return sc, nil
	case "path":
		// (path GLOB… body…) ≡ (under (path GLOB…) body…)
		globs, body, err := parsePathGlobs(list[1:])
		if err != nil {
			return sc, fmt.Errorf("%s: form %d: %w", path, i, err)
		}
		sc.paths = globs
		sc.hostLang = ""
		sc.lang = ""
		sc.embed = false
		sc.embedRegion = nil
		sc.nest = nil
		return loadExtractBodyForms(prog, env, body, sc, path, i)
	case "under":
		// (under PLACE BODY…) — PLACE is path, as-language, or a matcher place (node/…).
		if len(list) < 3 {
			return sc, fmt.Errorf("%w: %s: form %d: under wants place and body", ErrExtract, path, i)
		}
		scopeForm, ok := list[1].([]any)
		if !ok || len(scopeForm) == 0 {
			return sc, fmt.Errorf("%w: %s: form %d: under place must be a list form", ErrExtract, path, i)
		}
		sh, _ := scopeForm[0].(string)
		switch sh {
		case "path":
			globs, extra, err := parsePathGlobs(scopeForm[1:])
			if err != nil {
				return sc, fmt.Errorf("%s: form %d: path: %w", path, i, err)
			}
			if len(extra) > 0 {
				return sc, fmt.Errorf("%w: %s: form %d: path place must be only globs", ErrExtract, path, i)
			}
			child := sc
			child.paths = globs
			child.nest = nil
			child.hostLang = ""
			child.lang = ""
			child.embed = false
			child.embedRegion = nil
			_, err = loadExtractBodyForms(prog, env, list[2:], child, path, i)
			return sc, err
		case "as-language":
			if !sc.hasPath() {
				return sc, fmt.Errorf("%w: %s: form %d: as-language must be under (path …), not at root", ErrExtract, path, i)
			}
			if len(scopeForm) != 2 {
				return sc, fmt.Errorf("%w: %s: form %d: under (as-language ID) wants id only", ErrExtract, path, i)
			}
			id, err := extractAsString(scopeForm[1])
			if err != nil {
				return sc, fmt.Errorf("%s: form %d: as-language id (string): %w", path, i, err)
			}
			child, err := applyAsLanguage(sc, prog, id)
			if err != nil {
				return sc, fmt.Errorf("%s: form %d: %w", path, i, err)
			}
			_, err = loadExtractBodyForms(prog, env, list[2:], child, path, i)
			return sc, err
		default:
			// Matcher place: (under (node TYPE) …). Path optional for global paint packs.
			place, err := expand(scopeForm, env, 0)
			if err != nil {
				return sc, fmt.Errorf("%s: form %d: under place: %w", path, i, err)
			}
			child := sc
			child.nest = append(sc.cloneNest(), place)
			_, err = loadExtractBodyForms(prog, env, list[2:], child, path, i)
			return sc, err
		}
	case "as-language":
		// (as-language ID) or (as-language ID BODY…) — ID is a string language id.
		// Under path with empty nest → host. Under a place nest → embed (SPEC.md Regions).
		if !sc.hasPath() {
			return sc, fmt.Errorf("%w: %s: form %d: as-language must be inside (under (path …) …), not at root", ErrExtract, path, i)
		}
		if len(list) < 2 {
			return sc, fmt.Errorf("%w: %s: form %d: as-language wants id (string)", ErrExtract, path, i)
		}
		id, err := extractAsString(list[1])
		if err != nil {
			return sc, fmt.Errorf("%s: form %d: as-language id (string): %w", path, i, err)
		}
		sc, err = applyAsLanguage(sc, prog, id)
		if err != nil {
			return sc, fmt.Errorf("%s: form %d: %w", path, i, err)
		}
		if len(list) == 2 {
			return sc, nil
		}
		return loadExtractBodyForms(prog, env, list[2:], sc, path, i)
	case "as-family":
		if err := applyAsFamily(sc, prog, list, path, i); err != nil {
			return sc, err
		}
		return sc, nil
	case "as-layout":
		if err := applyAsLayout(sc, prog, list, path, i); err != nil {
			return sc, err
		}
		return sc, nil
	case "as-atomic":
		if err := applyAsAtomic(sc, prog, list, path, i); err != nil {
			return sc, err
		}
		return sc, nil
	case "as-docstring":
		if err := applyAsDocstring(sc, prog, list, path, i); err != nil {
			return sc, err
		}
		return sc, nil
	case "as-directory-representant":
		if err := applyAsDirectoryRepresentant(sc, prog, list, path, i); err != nil {
			return sc, err
		}
		return sc, nil
	case "as-grammar":
		if err := applyAsGrammar(sc, prog, list, path, i); err != nil {
			return sc, err
		}
		if len(list) == 2 {
			return sc, nil
		}
		return loadExtractBodyForms(prog, env, list[2:], sc, path, i)
	case "as-import":
		if hostTokenSeq(sc, list) {
			if err := applyAsImportSeq(sc, prog, list, path, i); err != nil {
				return sc, err
			}
			if len(list) == 2 {
				return sc, nil
			}
			return loadExtractBodyForms(prog, env, list[2:], sc, path, i)
		}
		fallthrough
	case "as-package":
		if hostTokenSeq(sc, list) {
			if err := applyAsPackageSeq(sc, prog, list, path, i); err != nil {
				return sc, err
			}
			if len(list) == 2 {
				return sc, nil
			}
			return loadExtractBodyForms(prog, env, list[2:], sc, path, i)
		}
		fallthrough
	case "as-atom", "as-use", "as-scope", "as-decl", "as-flow", "as-reexport", "as-default",
		"as-keyword", "as-string", "as-number", "as-comment",
		"as-type", "as-const", "as-ident", "as-op", "as-punct":
		// Place outside, verb inside. Graph verbs need path+host; paint verbs do not.
		paint := isPaintHead(head)
		if !paint {
			if !sc.hasPath() {
				return sc, fmt.Errorf("%w: %s: form %d: %s must be inside (under (path …) …)", ErrExtract, path, i, head)
			}
			if sc.lang == "" || sc.hostLang == "" {
				return sc, fmt.Errorf("%w: %s: form %d: %s needs host (as-language ID) under path first", ErrExtract, path, i, head)
			}
		}
		scopeField := ""
		var matchArg any
		args := list[1:]
		// (this) = current under place as MATCHER.
		// (scope CAP) = as-use from-where field (only on as-use).
		useThisPlace := false
		atomExported := false
		flowClass := ""
		if head == "as-use" {
			// (as-use) | (as-use MATCHER) | (as-use (scope CAP) …) | (as-use (scope CAP) MATCHER)
			for _, a := range args {
				if isThisPlace(a) {
					if matchArg != nil || useThisPlace {
						return sc, fmt.Errorf("%w: %s: form %d: as-use wants at most one MATCHER", ErrExtract, path, i)
					}
					useThisPlace = true
					continue
				}
				if sf, ok, err := parseScopeFieldForm(a); err != nil {
					return sc, fmt.Errorf("%s: form %d: %w", path, i, err)
				} else if ok {
					if scopeField != "" {
						return sc, fmt.Errorf("%w: %s: form %d: as-use: duplicate (scope CAP)", ErrExtract, path, i)
					}
					scopeField = sf
					continue
				}
				if matchArg != nil || useThisPlace {
					return sc, fmt.Errorf("%w: %s: form %d: as-use wants at most one MATCHER", ErrExtract, path, i)
				}
				if _, isStr := a.(string); isStr {
					return sc, fmt.Errorf("%w: %s: form %d: as-use wants a MATCHER form, not a string", ErrExtract, path, i)
				}
				matchArg = a
			}
		} else if head == "as-atom" {
			// (as-atom public|private M) — M may be (this). Bare / arity-1 fails.
			if len(args) != 2 {
				return sc, fmt.Errorf("%w: %s: form %d: as-atom wants public|private and MATCHER (got %d); bare (as-atom) is invalid", ErrExtract, path, i, len(args))
			}
			vis, err := parseAtomVisibility(args[0])
			if err != nil {
				return sc, fmt.Errorf("%w: %s: form %d: %v", ErrExtract, path, i, err)
			}
			atomExported = vis
			if isThisPlace(args[1]) {
				useThisPlace = true
			} else if _, ok := args[1].(string); ok {
				return sc, fmt.Errorf("%w: %s: form %d: as-atom wants a MATCHER form, not a string", ErrExtract, path, i)
			} else {
				matchArg = args[1]
			}
		} else if head == "as-scope" {
			// (as-scope M) only — bare form fails. M may be (this).
			if len(args) != 1 {
				return sc, fmt.Errorf("%w: %s: form %d: as-scope wants exactly one MATCHER (got %d); bare (as-scope) is invalid", ErrExtract, path, i, len(args))
			}
			if isThisPlace(args[0]) {
				useThisPlace = true
			} else if _, ok := args[0].(string); ok {
				return sc, fmt.Errorf("%w: %s: form %d: as-scope wants a MATCHER form, not a string", ErrExtract, path, i)
			} else {
				matchArg = args[0]
			}
		} else if head == "as-flow" {
			// (as-flow CLASS M) — CLASS is structural|hybrid; M may be (this).
			if len(args) != 2 {
				return sc, fmt.Errorf("%w: %s: form %d: as-flow wants CLASS and MATCHER (got %d)", ErrExtract, path, i, len(args))
			}
			cls, err := parseFlowClass(args[0])
			if err != nil {
				return sc, fmt.Errorf("%w: %s: form %d: %v", ErrExtract, path, i, err)
			}
			flowClass = cls
			if isThisPlace(args[1]) {
				useThisPlace = true
			} else if _, ok := args[1].(string); ok {
				return sc, fmt.Errorf("%w: %s: form %d: as-flow wants a MATCHER form, not a string", ErrExtract, path, i)
			} else {
				matchArg = args[1]
			}
		} else if head == "as-decl" {
			// (as-decl M) only — wrap is the first token of the hole span.
			if len(args) != 1 {
				return sc, fmt.Errorf("%w: %s: form %d: as-decl wants exactly one MATCHER (got %d)", ErrExtract, path, i, len(args))
			}
			if isThisPlace(args[0]) {
				useThisPlace = true
			} else if _, ok := args[0].(string); ok {
				return sc, fmt.Errorf("%w: %s: form %d: as-decl wants a MATCHER form, not a string", ErrExtract, path, i)
			} else {
				matchArg = args[0]
			}
		} else {
			switch len(args) {
			case 0:
				matchArg = nil
			case 1:
				if isThisPlace(args[0]) {
					useThisPlace = true
				} else if _, ok := args[0].(string); ok {
					return sc, fmt.Errorf("%w: %s: form %d: %s wants a MATCHER form, not a string", ErrExtract, path, i, head)
				} else {
					matchArg = args[0]
				}
			default:
				return sc, fmt.Errorf("%w: %s: form %d: %s wants optional MATCHER only (got %d args)", ErrExtract, path, i, head, len(args))
			}
		}
		if useThisPlace {
			// Matcher = innermost under place (same as historical bare-handler).
			if len(sc.nest) == 0 {
				return sc, fmt.Errorf("%w: %s: form %d: (this) needs an enclosing (under PLACE …)", ErrExtract, path, i)
			}
			matchArg = nil // wrapPlaceNest peels innermost nest as body
		}
		if matchArg != nil {
			var err error
			matchArg, err = expand(matchArg, env, 0)
			if err != nil {
				return sc, fmt.Errorf("%s: form %d: expand: %w", path, i, err)
			}
		}
		if (head == "as-atom" || head == "as-scope" || head == "as-decl" || head == "as-flow") && matchArg == nil && !useThisPlace {
			return sc, fmt.Errorf("%w: %s: form %d: %s wants a MATCHER", ErrExtract, path, i, head)
		}
		wrapped, err := wrapPlaceNest(sc.nest, matchArg)
		if err != nil {
			return sc, fmt.Errorf("%s: form %d: %s: %w", path, i, head, err)
		}
		if wrapped == nil && len(sc.nest) == 0 {
			return sc, fmt.Errorf("%w: %s: form %d: %s needs a place (under (node …) …) or matcher", ErrExtract, path, i, head)
		}
		act, err := compileExtractBody(wrapped)
		if err != nil {
			return sc, fmt.Errorf("%s: form %d: %w", path, i, err)
		}
		act.Paths = append([]string(nil), sc.paths...)
		act.HostLang = sc.hostLang
		act.Lang = sc.lang
		act.Embed = sc.embed
		act.NodeType = innermostNodeType(sc.nest)
		if isPaintHead(head) {
			src := matchArg
			if src == nil && len(sc.nest) > 0 {
				src = sc.nest[len(sc.nest)-1]
			}
			act.PaintLeaves = paintLeafNames(src)
		}
		if scopeField != "" {
			act.ScopeField = scopeField
			// Last under is the use site (identifier / call); peel it so
			// scope is the nearest def (method, not class, not the call).
			act.ScopeNode = scopeNodeForUse(sc.nest)
		}
		if sc.embed {
			regForm, err := wrapPlaceNest(sc.embedRegion, nil)
			if err != nil {
				return sc, fmt.Errorf("%s: form %d: embed region: %w", path, i, err)
			}
			regAct, err := compileExtractBody(regForm)
			if err != nil {
				return sc, fmt.Errorf("%s: form %d: embed region compile: %w", path, i, err)
			}
			act.Region = regAct.Matcher
		}
		switch head {
		case "as-package":
			act.Kind = ExtractPackage
		case "as-atom":
			act.Kind = ExtractAtom
			act.Exported = atomExported
		case "as-use":
			act.Kind = ExtractUse
		case "as-scope":
			act.Kind = ExtractScope
		case "as-decl":
			act.Kind = ExtractScope
			act.HoleOnly = true
		case "as-flow":
			act.Kind = ExtractFlow
			act.FlowClass = flowClass
		case "as-import":
			act.Kind = ExtractImport
		case "as-reexport":
			act.Kind = ExtractReexport
		case "as-default":
			act.Kind = ExtractDefault
		default:
			act.Kind = ExtractPaint
			act.TokenClass = paintTokenClass(head)
		}
		prog.Actions = append(prog.Actions, act)
		return sc, nil
	default:
		// Rewrite/rule/builtin/match: one VM, query = decode. Unknown extract heads ignored.
		return sc, nil
	}
}

func isPaintHead(head string) bool {
	switch head {
	case "as-keyword", "as-string", "as-number", "as-comment",
		"as-type", "as-const", "as-ident", "as-op", "as-punct":
		return true
	default:
		return false
	}
}

func paintTokenClass(head string) string {
	switch head {
	case "as-keyword":
		return ingest.HLKeyword
	case "as-string":
		return ingest.HLString
	case "as-number":
		return ingest.HLNumber
	case "as-comment":
		return ingest.HLComment
	case "as-type":
		return ingest.HLType
	case "as-const":
		return ingest.HLConst
	case "as-ident":
		return ingest.HLIdent
	case "as-op":
		return ingest.HLOp
	case "as-punct":
		return ingest.HLPunct
	default:
		return ingest.HLOther
	}
}

func loadExtractBodyForms(prog *ExtractProgram, env expandEnv, forms []any, sc extractScope, path string, i int) (extractScope, error) {
	for j, form := range forms {
		var err error
		sc, err = loadExtractTop(prog, env, form, sc, path, i*1000+j)
		if err != nil {
			return sc, err
		}
	}
	return sc, nil
}

func compileExtractBody(v any) (ExtractAction, error) {
	var act ExtractAction
	matchTree, take, err := peelTake(v)
	if err != nil {
		return act, err
	}
	act.Take = take
	matchTree = normalizePathUnderExtract(matchTree)
	m, err := MatcherFromExpanded(matchTree)
	if err != nil {
		return act, err
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		return act, err
	}
	act.Matcher = cm
	return act, nil
}

// peelTake pulls a single (take name body) wrapper; nested take is an error.
func peelTake(v any) (body any, take string, err error) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return v, "", nil
	}
	head, _ := list[0].(string)
	if head != "take" {
		return v, "", nil
	}
	if len(list) != 3 {
		return nil, "", fmt.Errorf("%w: take wants name and match", ErrExtract)
	}
	name, err := extractAsString(list[1])
	if err != nil {
		return nil, "", err
	}
	inner, innerTake, err := peelTake(list[2])
	if err != nil {
		return nil, "", err
	}
	if innerTake != "" {
		return nil, "", fmt.Errorf("%w: nested take", ErrExtract)
	}
	return inner, name, nil
}

func normalizePathUnderExtract(v any) any {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return v
	}
	head, _ := list[0].(string)
	if head == "under" && len(list) >= 3 {
		scope, ok := list[1].([]any)
		if ok && len(scope) == 2 {
			if sh, _ := scope[0].(string); sh == "path" {
				return []any{"path", scope[1], normalizePathUnderExtract(list[2])}
			}
		}
		out := make([]any, len(list))
		copy(out, list)
		out[2] = normalizePathUnderExtract(list[2])
		return out
	}
	if head == "path" && len(list) >= 3 {
		out := make([]any, len(list))
		copy(out, list)
		out[2] = normalizePathUnderExtract(list[2])
		return out
	}
	return v
}

func extractAsString(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: want string, got %T", ErrExtract, v)
	}
	return s, nil
}

func hostLanguageFrom(p *ExtractProgram, relPath string) (lang string, ok bool, err error) {
	rel := strings.TrimPrefix(filepathToSlash(relPath), "./")
	hosts := map[string]struct{}{}
	if p == nil {
		return "", false, nil
	}
	for _, act := range p.Actions {
		if act.Embed {
			continue
		}
		if !actionAcceptsPath(act.Paths, rel) {
			continue
		}
		h := act.HostLang
		if h == "" {
			// Global paint (empty path, empty host) does not attribute.
			continue
		}
		hosts[h] = struct{}{}
	}
	switch len(hosts) {
	case 0:
		return "", false, nil
	case 1:
		for h := range hosts {
			return h, true, nil
		}
	}
	var list []string
	for h := range hosts {
		list = append(list, h)
	}
	sort.Strings(list)
	return "", false, fmt.Errorf("%w: path %q claimed by multiple host languages: %s", ErrExtract, rel, strings.Join(list, ", "))
}

// ExtractInto writes one file's ingest tuples into st (stratum 1 + qualify).
func (p *ExtractProgram) ExtractInto(ctx context.Context, sess *project.Session, st *store.Store, root *sitter.Node, source []byte, relPath string) error {
	if p == nil {
		return fmt.Errorf("%w: extract program: nil", ErrExtract)
	}
	if st == nil {
		return fmt.Errorf("%w: extract: nil store", ErrExtract)
	}
	if root == nil {
		return fmt.Errorf("%w: extract: nil tree", ErrExtract)
	}
	rel := strings.TrimPrefix(filepathToSlash(relPath), "./")
	lang := p.hostLangFor(rel)
	st.Insert(store.RelationFile, store.Tuple{relPath, lang, ""})
	useID := 0
	pol := p.tapePolicy(lang)
	scratch := &fileScratch{pol: pol, forLang: p.tapePolicy, want: nodeTypesFromProgram(p, rel)}
	h := &ingestHost{ctx: ctx, st: st, sess: sess, p: p, root: root, source: source, fp: relPath, useID: &useID, scratch: scratch, pol: pol}
	if err := datalog.Eval(ctx, st, p.IngestClauses(relPath), h); err != nil {
		return err
	}
	if h.err != nil {
		return h.err
	}
	for _, act := range p.Actions {
		if !act.Embed || act.Matcher == nil {
			continue
		}
		if !actionAcceptsPath(act.Paths, rel) {
			continue
		}
		ms, err := matchEmbedAction(ctx, sess, relPath, source, root, act, scratch, p)
		if err != nil {
			return err
		}
		applyExtractMatches(st, relPath, &useID, p, act, source, root, ms, 0, scratch.nodes(root))
	}
	applyGuestExtractOnEmbeds(ctx, sess, st, p, rel, relPath, source, root, &useID, scratch)
	finishExtract(st, relPath, source)
	return nil
}

// ExtractErr runs the pack on a parsed host file (host tree + source).
func (p *ExtractProgram) ExtractErr(ctx context.Context, sess *project.Session, root *sitter.Node, source []byte, relPath string) (*project.FileExtract, error) {
	st := store.New()
	if err := p.ExtractInto(ctx, sess, st, root, source, relPath); err != nil {
		return nil, err
	}
	if view := store.ProjectExtract(st, relPath); view != nil {
		return view, nil
	}
	return &project.FileExtract{Path: relPath}, nil
}

func finishExtract(st *store.Store, fp string, source []byte) {
	if st == nil || fp == "" {
		return
	}
	scopes := scopesFromStore(st, fp)
	scopes = nestScopeParents(dedupScopeSpans(scopes))
	writeScopes(st, fp, scopes)

	atoms := atomsFromStore(st, fp)
	atoms = preferQualifiedAtoms(atoms)
	for i := range atoms {
		atoms[i].ScopeIdx = innermostScope(scopes, atoms[i].StartByte, atoms[i].EndByte)
	}
	qualifyAtomsFromScopes(atoms, scopes, source)
	writeAtoms(st, fp, atoms)

	uses := usesFromStore(st, fp)
	uses = preferScopedUsages(uses)
	for i := range uses {
		uses[i].ScopeIdx = innermostScope(scopes, uses[i].StartByte, uses[i].EndByte)
	}
	writeUses(st, fp, uses)
	st.ResetFile(store.RelationDeclares, fp)
	st.ResetFile(store.RelationAtomInFile, fp)
	store.WriteDeclares(st, fp)
}

func scopesFromStore(st *store.Store, fp string) []project.ScopeDef {
	max := -1
	for _, t := range st.Rows(store.RelationScope) {
		if len(t) < 5 || t[0] != fp {
			continue
		}
		if i := store.AtoiInt(t[1]); i > max {
			max = i
		}
	}
	if max < 0 {
		return nil
	}
	out := make([]project.ScopeDef, max+1)
	for _, t := range st.Rows(store.RelationScope) {
		if len(t) < 5 || t[0] != fp {
			continue
		}
		out[store.AtoiInt(t[1])] = store.ScopeFromRow(t)
	}
	return out
}

func atomsFromStore(st *store.Store, fp string) []project.AtomDef {
	var out []project.AtomDef
	for _, t := range st.Rows(store.RelationAtom) {
		if len(t) < 6 || t[0] != fp {
			continue
		}
		out = append(out, project.AtomDef{
			Name: t[1], StartByte: store.Atoi(t[2]), EndByte: store.Atoi(t[3]),
			Exported: t[4] == "1", ScopeIdx: store.AtoiInt(t[5]),
		})
	}
	return out
}

func usesFromStore(st *store.Store, fp string) []project.UsageDef {
	type segs []project.UsageName
	byID := map[string]segs{}
	for _, t := range st.Rows(store.RelationUseSegment) {
		if len(t) < 6 || t[0] != fp {
			continue
		}
		id, idx := t[1], store.AtoiInt(t[2])
		sl := byID[id]
		for len(sl) <= idx {
			sl = append(sl, project.UsageName{})
		}
		sl[idx] = project.UsageName{Name: t[3], StartByte: store.Atoi(t[4]), EndByte: store.Atoi(t[5])}
		byID[id] = sl
	}
	var out []project.UsageDef
	for _, t := range st.Rows(store.RelationUse) {
		if len(t) < 7 || t[0] != fp {
			continue
		}
		id := "0"
		if len(t) > 7 {
			id = t[7]
		}
		out = append(out, project.UsageDef{
			Name: t[1], StartByte: store.Atoi(t[2]), EndByte: store.Atoi(t[3]),
			Scope: t[4], ScopeIdx: store.AtoiInt(t[5]), Prefix: byID[id],
		})
	}
	return out
}

func writeScopes(st *store.Store, fp string, scopes []project.ScopeDef) {
	st.ResetFile(store.RelationScope, fp)
	for i, sc := range scopes {
		st.Insert(store.RelationScope, store.ScopeRow(fp, i, sc))
	}
}

func writeAtoms(st *store.Store, fp string, atoms []project.AtomDef) {
	st.ResetFile(store.RelationAtom, fp)
	for _, a := range atoms {
		st.Insert(store.RelationAtom, store.Tuple{
			fp, a.Name, store.Itoa(a.StartByte), store.Itoa(a.EndByte),
			bool01(a.Exported), store.ItoaInt(a.ScopeIdx),
		})
	}
}

func writeUses(st *store.Store, fp string, uses []project.UsageDef) {
	st.ResetFile(store.RelationUse, fp)
	st.ResetFile(store.RelationUseSegment, fp)
	for i, u := range uses {
		id := store.ItoaInt(i)
		st.Append(store.RelationUse, store.Tuple{
			fp, u.Name, store.Itoa(u.StartByte), store.Itoa(u.EndByte),
			u.Scope, store.ItoaInt(u.ScopeIdx), store.ItoaInt(len(u.Prefix)), id,
		})
		for j, p := range u.Prefix {
			st.Append(store.RelationUseSegment, store.Tuple{
				fp, id, store.ItoaInt(j), p.Name, store.Itoa(p.StartByte), store.Itoa(p.EndByte),
			})
		}
	}
}

func applyGuestExtractOnEmbeds(ctx context.Context, sess *project.Session, st *store.Store, p *ExtractProgram, rel, relPath string, source []byte, hostRoot *sitter.Node, useID *int, scratch *fileScratch) {
	if st == nil || p == nil {
		return
	}
	embs := uniqueEmbedRegions(ctx, sess, p, rel, relPath, source, hostRoot, scratch)
	for _, emb := range embs {
		applyLangExtract(ctx, sess, st, p, emb.root, emb.content, relPath, emb.lang, useID, emb.base)
		emb.close()
	}
}

func applyLangExtract(ctx context.Context, sess *project.Session, st *store.Store, p *ExtractProgram, root *sitter.Node, source []byte, relPath, lang string, useID *int, off uint32) {
	if st == nil || p == nil || root == nil || lang == "" {
		return
	}
	for _, act := range p.Actions {
		if act.Matcher == nil || act.Kind == ExtractPaint || act.Embed {
			continue
		}
		if !actionAppliesToLang(act, lang) {
			continue
		}
		ms, err := MatchFileMatcherPolicy(ctx, sess, ".", relPath, source, root, act.Matcher, nil, p.tapePolicy(lang))
		if err != nil {
			continue
		}
		applyExtractMatches(st, relPath, useID, p, act, source, root, ms, off, nil)
	}
}

func mergeExtractOffset(fe, sub *project.FileExtract, start uint32) {
	if fe == nil || sub == nil {
		return
	}
	for _, e := range sub.Atoms {
		e.StartByte += start
		e.EndByte += start
		fe.Atoms = append(fe.Atoms, e)
	}
	for _, im := range sub.Imports {
		im.StartByte += start
		im.EndByte += start
		if im.TargetStartByte != 0 || im.TargetEndByte != 0 {
			im.TargetStartByte += start
			im.TargetEndByte += start
		}
		fe.Imports = append(fe.Imports, im)
	}
	for _, u := range sub.Usages {
		u.StartByte += start
		u.EndByte += start
		for i := range u.Prefix {
			u.Prefix[i].StartByte += start
			u.Prefix[i].EndByte += start
		}
		fe.Usages = append(fe.Usages, u)
	}
	for _, r := range sub.Reexports {
		if r.SourceStartByte != 0 || r.SourceEndByte != 0 {
			r.SourceStartByte += start
			r.SourceEndByte += start
		}
		fe.Reexports = append(fe.Reexports, r)
	}
	if sub.DefaultExport != "" && fe.DefaultExport == "" {
		fe.DefaultExport = sub.DefaultExport
	}
	if fe.Package == "" && sub.Package != "" {
		fe.Package = sub.Package
	}
	for _, s := range sub.Scopes {
		s.StartByte += start
		s.EndByte += start
		fe.Scopes = append(fe.Scopes, s)
	}
	for _, fl := range sub.Flows {
		fl.StartByte += start
		fl.EndByte += start
		fe.Flows = append(fe.Flows, fl)
	}
}

// preferScopedUsages picks one use per span: prefer Scope set, then Prefix set.
func preferScopedUsages(us []project.UsageDef) []project.UsageDef {
	if len(us) < 2 {
		return us
	}
	type key struct{ s, e uint32 }
	best := make(map[key]project.UsageDef, len(us))
	order := make([]key, 0, len(us))
	better := func(a, b project.UsageDef) bool {
		// true if b should replace a. Prefix beats scope: Outer.Box
		// is the same token as a scoped bare Box.
		if len(a.Prefix) == 0 && len(b.Prefix) > 0 {
			return true
		}
		if len(a.Prefix) > 0 && len(b.Prefix) == 0 {
			return false
		}
		if a.Scope == "" && b.Scope != "" {
			return true
		}
		return false
	}
	for _, u := range us {
		k := key{u.StartByte, u.EndByte}
		prev, ok := best[k]
		if !ok {
			best[k] = u
			order = append(order, k)
			continue
		}
		if better(prev, u) {
			best[k] = u
		}
	}
	out := make([]project.UsageDef, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}
	return out
}

func matchEmbedAction(ctx context.Context, sess *project.Session, relPath string, source []byte, hostRoot *sitter.Node, act ExtractAction, scratch *fileScratch, p *ExtractProgram) ([]Match, error) {
	if act.Region == nil {
		return nil, fmt.Errorf("%w: embed action %s: missing region", ErrExtract, act.Lang)
	}
	if p == nil {
		return nil, fmt.Errorf("%w: embed action: nil extract program", ErrExtract)
	}
	hostPol := p.tapePolicy(p.hostLangFor(relPath))
	if scratch != nil {
		hostPol = scratch.pol
	}
	regs, err := matchFileMatcherPol(ctx, sess, ".", relPath, source, hostRoot, act.Region, nil, hostPol, scratch)
	if err != nil {
		return nil, err
	}
	gid := p.grammarID(act.Lang)
	if sess == nil || sess.Engine() == nil || !sess.Engine().Has(ctx, gid) {
		return nil, fmt.Errorf("%w: unknown language %q (grammar not registered)", ErrExtract, act.Lang)
	}
	var out []Match
	for _, reg := range regs {
		if reg.Empty() || int(reg.EndByte) > len(source) || reg.StartByte >= reg.EndByte {
			continue
		}
		raw := source[reg.StartByte:reg.EndByte]
		content, base := stripEmbedDelims(raw, reg.StartByte)
		if len(content) == 0 {
			continue
		}
		pf, err := ingestutil.ParseSource(ctx, sess.Engine(), content, relPath+"#"+act.Lang, gid)
		if err != nil {
			return nil, fmt.Errorf("embed %s: %w", act.Lang, err)
		}
		bodyMs, err := MatchFileMatcherPolicy(ctx, sess, ".", relPath, content, pf.Root, act.Matcher, nil, p.tapePolicy(act.Lang))
		pf.Close()
		if err != nil {
			return nil, err
		}
		for _, m := range bodyMs {
			out = append(out, offsetMatch(m, base))
		}
	}
	return out, nil
}

func applyExtractMatches(st *store.Store, fp string, useID *int, p *ExtractProgram, act ExtractAction, source []byte, root *sitter.Node, ms []Match, off uint32, idx *nodeIndex) {
	if st == nil {
		return
	}
	for _, m := range ms {
		locus := m.Span
		if act.Take != "" {
			var sp ingestutil.Span
			var ok bool
			// Rightmost name is the leaf (Type.method / recv.leaf), not slice order.
			if act.Take == "name" && (act.Kind == ExtractAtom || act.Kind == ExtractUse) {
				sp, ok = captureRightmostName(m)
			} else {
				sp, ok = m.CaptureFirst(act.Take)
			}
			if ok && !sp.Empty() {
				locus = sp
			} else {
				continue
			}
		}
		if locus.Empty() || int(locus.EndByte) > len(source) {
			continue
		}
		text := string(source[locus.StartByte:locus.EndByte])
		switch act.Kind {
		case ExtractPackage:
			st.Insert(store.RelationPackage, store.Tuple{fp, text, store.Itoa(locus.EndByte + off)})
		case ExtractAtom:
			for _, a := range atomDefsFromNames(m, source, locus, text, act.Exported) {
				st.Insert(store.RelationAtom, store.Tuple{
					fp, a.Name, store.Itoa(a.StartByte + off), store.Itoa(a.EndByte + off),
					bool01(a.Exported), store.ItoaInt(a.ScopeIdx),
				})
			}
		case ExtractUse:
			u := project.UsageDef{
				Name:      text,
				StartByte: locus.StartByte,
				EndByte:   locus.EndByte,
				ScopeIdx:  -1,
			}
			applyUseNameCaptures(&u, m, source, locus)
			if act.ScopeNode != "" && act.ScopeField != "" {
				u.Scope = enclosingNodeField(root, locus.StartByte, locus.EndByte, act.ScopeNode, act.ScopeField, source, idx)
			}
			if u.Scope == "" && act.ScopeField != "" && act.ScopeNode == "" {
				u.Scope = extractCapText(m, source, act.ScopeField)
			}
			if u.Scope == "" {
				u.Scope = extractCapText(m, source, "scope")
			}
			id := 0
			if useID != nil {
				id = *useID
				*useID++
			}
			st.Append(store.RelationUse, store.Tuple{
				fp, u.Name, store.Itoa(u.StartByte + off), store.Itoa(u.EndByte + off),
				u.Scope, store.ItoaInt(u.ScopeIdx), store.ItoaInt(len(u.Prefix)), store.ItoaInt(id),
			})
			for i, pfx := range u.Prefix {
				st.Append(store.RelationUseSegment, store.Tuple{
					fp, store.ItoaInt(id), store.ItoaInt(i), pfx.Name,
					store.Itoa(pfx.StartByte + off), store.Itoa(pfx.EndByte + off),
				})
			}
		case ExtractImport:
			if names := m.Captures["name"]; len(names) > 1 && extractCapText(m, source, "module_name") != "" {
				for _, nsp := range names {
					if nsp.Empty() {
						continue
					}
					writeExtractImport(st, fp, matchWithName(m, nsp), source, nsp.Text(source), nsp, off)
				}
				continue
			}
			writeExtractImport(st, fp, m, source, text, locus, off)
		case ExtractReexport:
			// Captures: path|source → SourcePath; name → SourceName;
			// alias|export → ExportName; star (or empty name with * in locus) → Star.
			pathText := extractCapText(m, source, "path")
			if pathText == "" {
				pathText = extractCapText(m, source, "source")
			}
			pathText = unquoteImportPath(pathText)
			sourceName := extractCapText(m, source, "name")
			exportName := extractCapText(m, source, "export")
			if exportName == "" {
				exportName = extractCapText(m, source, "alias")
			}
			star := extractCapText(m, source, "star") != "" ||
				(sourceName == "" && exportName == "" && pathText != "" && strings.Contains(text, "*"))
			if star {
				if pathText == "" {
					continue
				}
				st.Insert(store.RelationReexport, store.Tuple{fp, "", "", pathText, "1", "0", "0"})
				continue
			}
			if exportName == "" {
				exportName = sourceName
			}
			if sourceName == "" {
				sourceName = exportName
			}
			// Named reexport needs an export name; path may be empty (same-file forward).
			if exportName == "" {
				continue
			}
			var srcStart, srcEnd uint32
			if sp, ok := m.CaptureFirst("name"); ok && !sp.Empty() {
				srcStart, srcEnd = sp.StartByte, sp.EndByte
			} else {
				srcStart, srcEnd = locus.StartByte, locus.EndByte
			}
			st.Insert(store.RelationReexport, store.Tuple{
				fp, exportName, sourceName, pathText, "0",
				store.Itoa(srcStart + off), store.Itoa(srcEnd + off),
			})
		case ExtractDefault:
			// Module primary export:
			//   export default function X  → name field / take
			//   export { X as default }    → alias "default", name is X
			alias := extractCapText(m, source, "alias")
			name := extractCapText(m, source, "name")
			if alias != "" {
				if alias != "default" || name == "" {
					continue
				}
			} else if name == "" {
				name = text
			}
			if name == "" || name == "default" {
				continue
			}
			has := false
			for _, t := range st.Rows(store.RelationDefault) {
				if len(t) >= 2 && t[0] == fp {
					has = true
					break
				}
			}
			if !has {
				st.Insert(store.RelationDefault, store.Tuple{fp, name})
			}
		case ExtractScope:
			idx := 0
			for _, t := range st.Rows(store.RelationScope) {
				if len(t) > 0 && t[0] == fp {
					idx++
				}
			}
			st.Insert(store.RelationScope, store.ScopeRow(fp, idx, project.ScopeDef{
				Parent:    -1,
				StartByte: locus.StartByte + off,
				EndByte:   locus.EndByte + off,
				HoleOnly:  act.HoleOnly,
			}))
		case ExtractFlow:
			class := rewriteFlowClass(root, locus, act.FlowClass)
			if class == "" {
				continue
			}
			st.Insert(store.RelationFlow, store.Tuple{
				fp, store.Itoa(locus.StartByte + off), store.Itoa(locus.EndByte + off), class,
			})
		}
	}
}

func bool01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func parseFlowClass(v any) (string, error) {
	s, err := extractAsString(v)
	if err != nil {
		return "", fmt.Errorf("as-flow CLASS: %w", err)
	}
	switch s {
	case project.FlowStructural, project.FlowHybrid:
		return s, nil
	default:
		return "", fmt.Errorf("as-flow CLASS must be structural or hybrid, got %q", s)
	}
}

func isIfLike(typ string) bool {
	switch typ {
	case "if_statement", "if_expression", "if_let_expression":
		return true
	default:
		return strings.HasPrefix(typ, "if_")
	}
}

// rewriteFlowClass applies the else-if host rule:
//   - structural consequence of an if that is some other if's alternative → hybrid
//   - hybrid whose span is that inner if (the whole alternative) → drop
func rewriteFlowClass(root *sitter.Node, loc ingestutil.Span, class string) string {
	if root == nil || class == "" {
		return class
	}
	var altIf, consequenceOfAltIf bool
	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil || n.IsNull() {
			return
		}
		if n.EndByte() <= loc.StartByte || n.StartByte() >= loc.EndByte {
			return
		}
		for i := uint32(0); i < n.ChildCount(); i++ {
			c := n.Child(i)
			if c == nil || c.IsNull() {
				continue
			}
			if n.FieldNameForChild(i) == "alternative" && isIfLike(c.Type()) {
				if c.StartByte() == loc.StartByte && c.EndByte() == loc.EndByte {
					altIf = true
				}
				if cons := ingestutil.ChildByField(c, "consequence"); cons != nil &&
					cons.StartByte() == loc.StartByte && cons.EndByte() == loc.EndByte {
					consequenceOfAltIf = true
				}
			}
			walk(c)
		}
	}
	walk(root)
	if class == project.FlowHybrid && altIf {
		return ""
	}
	if class == project.FlowStructural && consequenceOfAltIf {
		return project.FlowHybrid
	}
	return class
}

func shiftSpan(start, end, off uint32) (uint32, uint32) {
	if end <= start {
		return start, end
	}
	return start + off, end + off
}

type namePart struct {
	span ingestutil.Span
	text string
}

func captureRightmostName(m Match) (ingestutil.Span, bool) {
	var best ingestutil.Span
	ok := false
	for _, sp := range m.Captures["name"] {
		if sp.Empty() {
			continue
		}
		if !ok || sp.EndByte > best.EndByte {
			best, ok = sp, true
		}
	}
	return best, ok
}

func namePartsFromMatch(m Match, source []byte) []namePart {
	var parts []namePart
	seenSpan := map[[2]uint32]bool{}
	for _, sp := range m.Captures["name"] {
		if sp.Empty() || int(sp.EndByte) > len(source) {
			continue
		}
		t := stripSpaces(string(source[sp.StartByte:sp.EndByte]))
		if !identNameText(t) || isPatternSoupName(t) {
			continue
		}
		k := [2]uint32{sp.StartByte, sp.EndByte}
		if seenSpan[k] {
			continue
		}
		seenSpan[k] = true
		parts = append(parts, namePart{span: sp, text: t})
	}
	sort.SliceStable(parts, func(i, j int) bool {
		if parts[i].span.StartByte != parts[j].span.StartByte {
			return parts[i].span.StartByte < parts[j].span.StartByte
		}
		return parts[i].span.EndByte < parts[j].span.EndByte
	})
	return parts
}

// namePathJoinMid reports whether source between two name captures is a
// selector, so they are one reference. Call-chain mids ((). [] ) are not.
func namePathJoinMid(s string) bool {
	t := strings.TrimSpace(s)
	return t == "" || t == "." || t == "::" || t == ":"
}

// applyUseNameCaptures sets leaf + prefix from name-field captures.
// Last ident inside locus is the use token; earlier captures join only
// when the mid is empty / . / :: / : (the node already is that spelling).
func applyUseNameCaptures(u *project.UsageDef, m Match, source []byte, locus ingestutil.Span) {
	var parts []namePart
	seen := map[[2]uint32]bool{}
	for _, key := range []string{"name", "field", "attribute", "object", "operand", "array", "function"} {
		for _, sp := range m.Captures[key] {
			if sp.Empty() || int(sp.EndByte) > len(source) {
				continue
			}
			t := stripSpaces(string(source[sp.StartByte:sp.EndByte]))
			if t == "" || isPatternSoupName(t) || !isSingleIdent(t) {
				continue
			}
			k := [2]uint32{sp.StartByte, sp.EndByte}
			if seen[k] {
				continue
			}
			seen[k] = true
			parts = append(parts, namePart{span: sp, text: t})
		}
	}
	sort.SliceStable(parts, func(i, j int) bool {
		if parts[i].span.StartByte != parts[j].span.StartByte {
			return parts[i].span.StartByte < parts[j].span.StartByte
		}
		return parts[i].span.EndByte < parts[j].span.EndByte
	})
	leafIdx := -1
	for i, p := range parts {
		if !identNameText(p.text) {
			continue
		}
		if p.span.StartByte >= locus.StartByte && p.span.EndByte <= locus.EndByte {
			leafIdx = i
		}
	}
	if leafIdx < 0 {
		return
	}
	leaf := parts[leafIdx]
	u.Name = leaf.text
	u.StartByte = leaf.span.StartByte
	u.EndByte = leaf.span.EndByte
	end := leaf.span.StartByte
	var prefs []namePart
	for i := leafIdx - 1; i >= 0; i-- {
		p := parts[i]
		// `any` may also bind the whole Outer.Box node; skip it.
		if p.span.StartByte <= leaf.span.StartByte && p.span.EndByte >= leaf.span.EndByte {
			continue
		}
		// this.name / (T) o).name — not a type qualifier.
		if !identNameText(p.text) || p.text == "this" || p.text == "super" {
			continue
		}
		if p.span.EndByte > end {
			break
		}
		mid := string(source[p.span.EndByte:end])
		if !namePathJoinMid(mid) {
			break
		}
		prefs = append([]namePart{p}, prefs...)
		end = p.span.StartByte
	}
	if len(prefs) == 0 {
		return
	}
	u.Prefix = make([]project.UsageName, len(prefs))
	for i, p := range prefs {
		u.Prefix[i] = project.UsageName{
			Name:      p.text,
			StartByte: p.span.StartByte,
			EndByte:   p.span.EndByte,
		}
	}
}

// atomDefsFromNames builds atoms from "name" captures.
// Comma-separated names on one spec (const a, b) each become an atom.
// Names separated by other source (func (T) M, type T { F) join as Type.leaf.
func atomDefsFromNames(m Match, source []byte, locus ingestutil.Span, locusText string, exported bool) []project.AtomDef {
	parts := namePartsFromMatch(m, source)
	if len(parts) == 0 {
		if isPatternSoupName(locusText) {
			return nil
		}
		return []project.AtomDef{atomDef(locusText, locus, exported)}
	}
	// Group: list-separators start a new atom; otherwise join onto the path.
	var groups [][]namePart
	groups = append(groups, []namePart{parts[0]})
	for i := 1; i < len(parts); i++ {
		prev, cur := parts[i-1], parts[i]
		mid := ""
		if cur.span.StartByte >= prev.span.EndByte && int(cur.span.StartByte) <= len(source) {
			mid = string(source[prev.span.EndByte:cur.span.StartByte])
		}
		if nameListSeparator(mid) {
			groups = append(groups, []namePart{cur})
			continue
		}
		g := groups[len(groups)-1]
		// Drop doubled leaf (node field + seq on the same span).
		// Distinct spans with the same text stay (Type.Type, Helper.Helper).
		if len(g) > 0 && g[len(g)-1].text == cur.text &&
			g[len(g)-1].span.Overlaps(cur.span) {
			g[len(g)-1] = cur
			groups[len(groups)-1] = g
			continue
		}
		groups[len(groups)-1] = append(g, cur)
	}
	out := make([]project.AtomDef, 0, len(groups))
	for _, g := range groups {
		leaf := g[len(g)-1]
		var texts []string
		seen := map[string]bool{}
		for _, p := range g[:len(g)-1] {
			if seen[p.text] {
				continue
			}
			if p.text == leaf.text && p.span.Overlaps(leaf.span) {
				continue
			}
			seen[p.text] = true
			texts = append(texts, p.text)
		}
		texts = append(texts, leaf.text)
		out = append(out, atomDef(strings.Join(texts, "."), leaf.span, exported))
	}
	return out
}

func atomDef(name string, sp ingestutil.Span, exported bool) project.AtomDef {
	return project.AtomDef{Name: name, StartByte: sp.StartByte, EndByte: sp.EndByte, Exported: exported, ScopeIdx: -1}
}

func parseAtomVisibility(v any) (bool, error) {
	s, err := extractAsString(v)
	if err != nil {
		return false, fmt.Errorf("as-atom wants public or private")
	}
	switch s {
	case "public":
		return true, nil
	case "private":
		return false, nil
	default:
		return false, fmt.Errorf("as-atom wants public or private, got %q", s)
	}
}

// isPatternSoupName is a destructure / object pattern taken as one name
// (const [a, b] = …). Computed method names like [Symbol.asyncDispose] stay.
func isPatternSoupName(s string) bool {
	s = stripSpaces(s)
	if s == "" {
		return false
	}
	if s[0] == '{' {
		return true
	}
	if s[0] == '[' && len(s) >= 2 && s[len(s)-1] == ']' {
		inner := stripSpaces(s[1 : len(s)-1])
		if isSingleIdent(inner) {
			return true
		}
		return strings.Contains(inner, ",")
	}
	return false
}

func isSingleIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r == '$':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func identNameText(s string) bool {
	if s == "" || s == "," {
		return false
	}
	for _, r := range s {
		if r == '_' || r == '$' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func nameListSeparator(s string) bool {
	if s == "" {
		return true
	}
	sawComma := false
	for _, r := range s {
		switch r {
		case ',':
			sawComma = true
		case ' ', '\t', '\n', '\r':
		default:
			return false
		}
	}
	return sawComma
}

// nestAndAssignScopes dedups equal as-scope spans, sets Parent by containment,
// and puts each atom/use on the innermost covering scope (-1 = file).
func nestAndAssignScopes(fe *project.FileExtract) {
	if fe == nil {
		return
	}
	fe.Scopes = nestScopeParents(dedupScopeSpans(fe.Scopes))
	for i := range fe.Atoms {
		fe.Atoms[i].ScopeIdx = innermostScope(fe.Scopes, fe.Atoms[i].StartByte, fe.Atoms[i].EndByte)
	}
	for i := range fe.Usages {
		fe.Usages[i].ScopeIdx = innermostScope(fe.Scopes, fe.Usages[i].StartByte, fe.Usages[i].EndByte)
	}
}

func atomLeaf(name string) string {
	if i := lastPathDot(name); i >= 0 {
		return name[i+1:]
	}
	return name
}

// lastPathDot is the last "." that is a Type.leaf join, not a dot inside
// [Symbol.asyncDispose] or a quoted leaf (Type.'.md').
func lastPathDot(name string) int {
	depth := 0
	var q byte
	last := -1
	for i := 0; i < len(name); i++ {
		c := name[i]
		if q != 0 {
			if c == q {
				q = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			q = c
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '.':
			if depth == 0 {
				last = i
			}
		}
	}
	return last
}

// wordBefore is the identifier immediately left of pos (skip whitespace).
func wordBefore(source []byte, pos int) string {
	if pos > len(source) {
		pos = len(source)
	}
	i := pos
	for i > 0 {
		c := source[i-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i--
			continue
		}
		break
	}
	end := i
	for i > 0 {
		c := source[i-1]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' {
			i--
			continue
		}
		break
	}
	if i == end {
		return ""
	}
	return string(source[i:end])
}

func isFunWord(w string) bool {
	switch w {
	case "fun", "fn", "func", "def":
		return true
	}
	return false
}

func isTypeWord(w string) bool {
	switch w {
	case "class", "struct", "enum", "interface", "object", "trait", "record", "impl":
		return true
	}
	return false
}

// qualifyAtomsFromScopes joins Type.leaf from the scope stack.
// First atom in a nested scope takes the enclosing scope's atom name
// (deinit inside TargaRLEDecoder, TypeAdapters.read inside URI).
// Later atoms in an as-decl (hole) are members (Type.field).
// Later atoms in as-scope (function) stay locals (self).
// Class as-scope later atoms are Type.field; first atoms stay local
// (Builder under CodeBlock) unless they are fun/fn/def (Type.method).
func qualifyAtomsFromScopes(atoms []project.AtomDef, scopes []project.ScopeDef, source []byte) {
	if len(atoms) == 0 {
		return
	}
	first := make(map[int]int, len(scopes))
	for i, a := range atoms {
		if a.ScopeIdx < 0 {
			continue
		}
		j, ok := first[a.ScopeIdx]
		if !ok || a.StartByte < atoms[j].StartByte {
			first[a.ScopeIdx] = i
		}
	}
	depth := func(idx int) int {
		n := 0
		for idx >= 0 && idx < len(scopes) {
			n++
			idx = scopes[idx].Parent
		}
		return n
	}
	order := make([]int, len(atoms))
	for i := range atoms {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return depth(atoms[order[i]].ScopeIdx) > depth(atoms[order[j]].ScopeIdx)
	})
	scopeIsType := func(idx int) bool {
		if idx < 0 || idx >= len(scopes) {
			return false
		}
		if scopes[idx].HoleOnly {
			return true
		}
		j, ok := first[idx]
		if !ok {
			return false
		}
		return isTypeWord(wordBefore(source, int(atoms[j].StartByte)))
	}
	typePath := func(idx int) string {
		var parts []string
		for idx >= 0 && idx < len(scopes) {
			if scopeIsType(idx) {
				if j, ok := first[idx]; ok && atoms[j].Name != "" {
					parts = append([]string{atomLeaf(atoms[j].Name)}, parts...)
				}
			}
			idx = scopes[idx].Parent
		}
		return strings.Join(parts, ".")
	}
	for _, i := range order {
		a := &atoms[i]
		if a.ScopeIdx < 0 || first[a.ScopeIdx] != i {
			continue
		}
		p := scopes[a.ScopeIdx].Parent
		j, ok := first[p]
		if !ok {
			continue
		}
		owner := atoms[j].Name
		if owner == "" {
			continue
		}
		ql := atomQualifier(a.Name)
		leaf := atomLeaf(owner)
		if ql == owner || ql == leaf || strings.HasSuffix(ql, "."+leaf) {
			continue
		}
		// Bare leaf + undotted owner: as-decl parent (zig Type.method) or
		// fun/fn/def under a type as-scope (Kotlin Type.method).
		// Type as-scope first atoms stay local (Builder under CodeBlock).
		if ql == "" && lastPathDot(owner) < 0 && !scopes[p].HoleOnly {
			if !scopeIsType(p) || !isFunWord(wordBefore(source, int(a.StartByte))) {
				continue
			}
			if path := typePath(p); path != "" {
				a.Name = path + "." + atomLeaf(a.Name)
			}
			continue
		}
		a.Name = leaf + "." + atomLeaf(a.Name)
	}
	for i := range atoms {
		a := &atoms[i]
		if a.ScopeIdx < 0 || first[a.ScopeIdx] == i {
			continue
		}
		if scopes[a.ScopeIdx].HoleOnly {
			owner := atoms[first[a.ScopeIdx]].Name
			if owner == "" {
				continue
			}
			if a.Name == owner || strings.HasPrefix(a.Name, owner+".") {
				continue
			}
			a.Name = owner + "." + atomLeaf(a.Name)
			continue
		}
		if !scopeIsType(a.ScopeIdx) {
			continue
		}
		// Class as-scope later atoms: Kotlin val/var properties.
		// Destructors / nested types stay on their own atom (bare ~AbsExtension).
		w := wordBefore(source, int(a.StartByte))
		if w != "val" && w != "var" {
			continue
		}
		path := typePath(a.ScopeIdx)
		if path == "" || a.Name == path || strings.HasPrefix(a.Name, path+".") {
			continue
		}
		a.Name = path + "." + atomLeaf(a.Name)
	}
}

func atomQualifier(name string) string {
	if i := lastPathDot(name); i >= 0 {
		return name[:i]
	}
	return ""
}

func dedupScopeSpans(in []project.ScopeDef) []project.ScopeDef {
	if len(in) < 2 {
		for i := range in {
			in[i].Parent = -1
		}
		return in
	}
	type key struct{ s, e uint32 }
	seen := make(map[key]struct{}, len(in))
	out := make([]project.ScopeDef, 0, len(in))
	for _, s := range in {
		if s.EndByte <= s.StartByte {
			continue
		}
		k := key{s.StartByte, s.EndByte}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		s.Parent = -1
		out = append(out, s)
	}
	return out
}

func nestScopeParents(scopes []project.ScopeDef) []project.ScopeDef {
	for i := range scopes {
		parent := -1
		for j := range scopes {
			if i == j {
				continue
			}
			if !spanContains(scopes[j], scopes[i].StartByte, scopes[i].EndByte) {
				continue
			}
			if parent == -1 || spanContains(scopes[parent], scopes[j].StartByte, scopes[j].EndByte) {
				parent = j
			}
		}
		scopes[i].Parent = parent
	}
	return scopes
}

func innermostScope(scopes []project.ScopeDef, start, end uint32) int {
	best := -1
	var bestLen uint32
	for i, s := range scopes {
		if s.StartByte > start || end > s.EndByte {
			continue
		}
		ln := s.EndByte - s.StartByte
		if best == -1 || ln < bestLen {
			best = i
			bestLen = ln
		}
	}
	return best
}

func spanContains(s project.ScopeDef, start, end uint32) bool {
	return s.StartByte <= start && end <= s.EndByte &&
		(s.StartByte < start || end < s.EndByte)
}

// preferQualifiedAtoms keeps one atom per span; longer (more dots) Name wins.
func preferQualifiedAtoms(as []project.AtomDef) []project.AtomDef {
	if len(as) < 2 {
		return as
	}
	type key struct{ s, e uint32 }
	best := make(map[key]project.AtomDef, len(as))
	order := make([]key, 0, len(as))
	for _, a := range as {
		k := key{a.StartByte, a.EndByte}
		prev, ok := best[k]
		if !ok {
			best[k] = a
			order = append(order, k)
			continue
		}
		if strings.Count(a.Name, ".") > strings.Count(prev.Name, ".") ||
			(strings.Count(a.Name, ".") == strings.Count(prev.Name, ".") && len(a.Name) > len(prev.Name)) {
			best[k] = a
		}
	}
	out := make([]project.AtomDef, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}
	return out
}

// scopeNodeForUse peels the last under (use site) and returns the innermost
// remaining node type (the method, not the class and not the call).
func scopeNodeForUse(nest []any) string {
	if len(nest) > 1 {
		nest = nest[:len(nest)-1]
	}
	return innermostNodeType(nest)
}

func matchWithName(m Match, nsp ingestutil.Span) Match {
	nm := m
	caps := make(map[string][]ingestutil.Span, len(m.Captures))
	for k, v := range m.Captures {
		caps[k] = v
	}
	caps["name"] = []ingestutil.Span{nsp}
	delete(caps, "alias")
	nm.Captures = caps
	nm.Span = nsp
	return nm
}

func writeExtractImport(st *store.Store, fp string, m Match, source []byte, text string, locus ingestutil.Span, off uint32) {
	pathText := extractCapText(m, source, "path")
	if pathText == "" {
		pathText = extractCapText(m, source, "module_name")
	}
	if pathText == "" {
		pathText = extractCapText(m, source, "source")
	}
	if pathText == "" {
		pathText = text
	}
	pathText = unquoteImportPath(pathText)

	nameText := extractCapText(m, source, "name")
	aliasText := extractCapText(m, source, "alias")
	modField := extractCapText(m, source, "module_name")
	local := text
	member := ""
	if modField == "" && nameText != "" && (pathText == text || pathText == "") {
		pathText = unquoteImportPath(nameText)
		if aliasText != "" {
			local = aliasText
		} else if i := strings.Index(nameText, " as "); i >= 0 {
			pathText = unquoteImportPath(strings.TrimSpace(nameText[:i]))
			local = strings.TrimSpace(nameText[i+4:])
		} else {
			local = ingest.LastPathComponent(pathText)
		}
	} else if nameText != "" {
		if aliasText != "" {
			member = nameText
			local = aliasText
		} else if i := strings.Index(nameText, " as "); i >= 0 {
			member = strings.TrimSpace(nameText[:i])
			local = strings.TrimSpace(nameText[i+4:])
		} else {
			local = nameText
			if pathText != "" && pathText != text {
				member = nameText
			}
		}
	}
	if local == pathText || local == "" || strings.HasPrefix(local, "\"") {
		local = ingest.LastPathComponent(pathText)
	}

	var pathStart, pathEnd uint32
	if sp, ok := m.CaptureFirst("path"); ok && !sp.Empty() {
		pathStart, pathEnd = stripImportPathQuotes(source, sp.StartByte, sp.EndByte)
	} else if sp, ok := m.CaptureFirst("module_name"); ok && !sp.Empty() {
		pathStart, pathEnd = sp.StartByte, sp.EndByte
	} else if sp, ok := m.CaptureFirst("source"); ok && !sp.Empty() {
		pathStart, pathEnd = stripImportPathQuotes(source, sp.StartByte, sp.EndByte)
	}

	start, end := locus.StartByte, locus.EndByte
	var targetStart, targetEnd uint32
	hasAlias := false
	if sp, ok := m.CaptureFirst("alias"); ok && !sp.Empty() {
		start, end = sp.StartByte, sp.EndByte
		hasAlias = true
		if nsp, ok := m.CaptureFirst("name"); ok && !nsp.Empty() {
			targetStart, targetEnd = nsp.StartByte, nsp.EndByte
		}
	} else if sp, ok := m.CaptureFirst("name"); ok && !sp.Empty() {
		start, end = sp.StartByte, sp.EndByte
		if member != "" && local != member {
			seg := string(source[start:end])
			if i := strings.LastIndex(seg, " as "); i >= 0 {
				hasAlias = true
				targetStart, targetEnd = start, start+uint32(i)
				for targetEnd > targetStart && source[targetEnd-1] == ' ' {
					targetEnd--
				}
				start = start + uint32(i+4)
				for start < end && (source[start] == ' ' || source[start] == '\t') {
					start++
				}
			}
		}
	} else if sp, ok := m.CaptureFirst("path"); ok && !sp.Empty() {
		start, end = sp.StartByte, sp.EndByte
		if end-start >= 2 {
			raw := source[start:end]
			if (raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '`' && raw[len(raw)-1] == '`') {
				start++
				end--
			}
		}
	} else if sp, ok := m.CaptureFirst("module_name"); ok && !sp.Empty() {
		start, end = sp.StartByte, sp.EndByte
	}
	if hasAlias && targetEnd == 0 && member != "" {
		if i := strings.Index(string(source[locus.StartByte:locus.EndByte]), member); i >= 0 {
			targetStart = locus.StartByte + uint32(i)
			targetEnd = targetStart + uint32(len(member))
		}
	}
	star := member == "*" || local == "*"
	// Path-only site (import "fmt"): s/e is the statement line so dead-import
	// deletes the spec, not the specifier (import ""). Named members keep
	// the name span. Path stays on ps/pe.
	if !hasAlias && member == "" && !star && !locus.Empty() {
		start, end = expandToImportLine(source, locus.StartByte, locus.EndByte)
	}
	ts, te := shiftSpan(targetStart, targetEnd, off)
	ps, pe := shiftSpan(pathStart, pathEnd, off)
	st.Insert(store.RelationImport, store.Tuple{
		fp, local, pathText, member,
		store.Itoa(start + off), store.Itoa(end + off),
		store.Itoa(ts), store.Itoa(te),
		bool01(hasAlias),
		store.Itoa(ps), store.Itoa(pe),
		bool01(star),
	})
}

func expandToImportLine(content []byte, start, end uint32) (uint32, uint32) {
	if end <= start || int(end) > len(content) {
		return start, end
	}
	for start > 0 && content[start-1] != '\n' {
		start--
	}
	for end < uint32(len(content)) && content[end] != '\n' {
		end++
	}
	if end < uint32(len(content)) && content[end] == '\n' {
		end++
	}
	return start, end
}

func extractCapText(m Match, source []byte, name string) string {
	sp, ok := m.CaptureFirst(name)
	if !ok || sp.Empty() || int(sp.EndByte) > len(source) {
		return ""
	}
	return string(source[sp.StartByte:sp.EndByte])
}

// isThisPlace is (this) — matcher shorthand for the enclosing under place.
func isThisPlace(v any) bool {
	list, ok := v.([]any)
	if !ok || len(list) != 1 {
		return false
	}
	h, _ := list[0].(string)
	return h == "this"
}

// parseScopeFieldForm recognizes (scope CAPTURE-NAME) for as-use from-where.
func parseScopeFieldForm(v any) (field string, ok bool, err error) {
	list, is := v.([]any)
	if !is || len(list) == 0 {
		return "", false, nil
	}
	h, _ := list[0].(string)
	if h != "scope" {
		return "", false, nil
	}
	if len(list) != 2 {
		return "", true, fmt.Errorf("%w: scope wants one capture name", ErrExtract)
	}
	s, err := extractAsString(list[1])
	if err != nil {
		return "", true, fmt.Errorf("scope name: %w", err)
	}
	if s == "" {
		return "", true, fmt.Errorf("%w: scope name empty", ErrExtract)
	}
	return s, true, nil
}

// outermostNodeType returns the type string of the first (node TYPE) in nest.
func outermostNodeType(nest []any) string {
	for _, p := range nest {
		if s := nestNodeType(p); s != "" {
			return s
		}
	}
	return ""
}

func innermostNodeType(nest []any) string {
	for i := len(nest) - 1; i >= 0; i-- {
		if s := nestNodeType(nest[i]); s != "" {
			return s
		}
	}
	return ""
}

func nestNodeType(p any) string {
	list, ok := p.([]any)
	if !ok || len(list) < 2 {
		return ""
	}
	h, _ := list[0].(string)
	if h != "node" {
		return ""
	}
	s, err := extractAsString(list[1])
	if err != nil {
		return ""
	}
	return s
}

// enclosingNodeField finds an ancestor of type nodeType covering [start,end)
// and returns its def-like name: named field (e.g. name / declarator), else the
// first direct identifier-like child (Kotlin simple_identifier without a field).
func enclosingNodeField(root *sitter.Node, start, end uint32, nodeType, field string, source []byte, idx *nodeIndex) string {
	if s := idx.enclosingField(start, end, nodeType, field, source); s != "" {
		return s
	}
	if root == nil || nodeType == "" {
		return ""
	}
	path := ancestorsCovering(root, start, end)
	for i := len(path) - 1; i >= 0; i-- {
		n := path[i]
		if n.Type() != nodeType {
			continue
		}
		if s := defNameFromNode(n, field, source); s != "" {
			return s
		}
	}
	return ""
}

// defNameFromNode extracts a definition name from a declaration node.
func defNameFromNode(n *sitter.Node, field string, source []byte) string {
	if n == nil {
		return ""
	}
	if field != "" {
		if ch := fieldChild(n, field); ch != nil {
			if id := deepestIdent(ch); id != nil {
				return nodeText(id, source)
			}
			return nodeText(ch, source)
		}
	}
	// Prefer standard "name" field, then C-style "declarator".
	for _, f := range []string{"name", "declarator"} {
		if f == field {
			continue
		}
		if ch := fieldChild(n, f); ch != nil {
			if id := deepestIdent(ch); id != nil {
				return nodeText(id, source)
			}
		}
	}
	// Kotlin etc.: first direct identifier-like child (before body).
	for i := uint32(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c == nil || c.IsNull() {
			continue
		}
		if isIdentNodeType(c.Type()) {
			return nodeText(c, source)
		}
	}
	return ""
}

func nodeText(n *sitter.Node, source []byte) string {
	if n == nil {
		return ""
	}
	sb, eb := n.StartByte(), n.EndByte()
	if int(eb) > len(source) || sb >= eb {
		return ""
	}
	return string(source[sb:eb])
}

func isIdentNodeType(t string) bool {
	switch t {
	case "identifier", "type_identifier", "field_identifier",
		"simple_identifier", "package_identifier", "property_identifier":
		return true
	default:
		return false
	}
}

// deepestIdent finds the leftmost deepest identifier-like node under n.
func deepestIdent(n *sitter.Node) *sitter.Node {
	if n == nil || n.IsNull() {
		return nil
	}
	if isIdentNodeType(n.Type()) {
		return n
	}
	for i := uint32(0); i < n.ChildCount(); i++ {
		if id := deepestIdent(n.Child(i)); id != nil {
			return id
		}
	}
	return nil
}

// ancestorsCovering returns root→…→innermost nodes that cover [start,end).
func ancestorsCovering(root *sitter.Node, start, end uint32) []*sitter.Node {
	if root == nil || root.IsNull() {
		return nil
	}
	if root.StartByte() > start || root.EndByte() < end {
		return nil
	}
	var path []*sitter.Node
	n := root
	for {
		path = append(path, n)
		var next *sitter.Node
		for i := uint32(0); i < n.ChildCount(); i++ {
			c := n.Child(i)
			if c == nil || c.IsNull() {
				continue
			}
			if c.StartByte() <= start && c.EndByte() >= end {
				if next == nil || (c.EndByte()-c.StartByte()) < (next.EndByte()-next.StartByte()) {
					next = c
				}
			}
		}
		if next == nil {
			return path
		}
		n = next
	}
}

func fieldChild(n *sitter.Node, field string) *sitter.Node {
	if n == nil || field == "" {
		return nil
	}
	for i := uint32(0); i < n.ChildCount(); i++ {
		if n.FieldNameForChild(i) == field {
			return n.Child(i)
		}
	}
	return nil
}

func stripSpaces(s string) string {
	return strings.ReplaceAll(s, " ", "")
}

func stripImportPathQuotes(source []byte, start, end uint32) (uint32, uint32) {
	if end < start+2 || int(end) > len(source) {
		return start, end
	}
	raw := source[start:end]
	if (raw[0] == '"' && raw[len(raw)-1] == '"') ||
		(raw[0] == '`' && raw[len(raw)-1] == '`') ||
		(raw[0] == '\'' && raw[len(raw)-1] == '\'') {
		return start + 1, end - 1
	}
	return start, end
}

func unquoteImportPath(s string) string {
	if len(s) >= 2 {
		switch {
		case s[0] == '"' && s[len(s)-1] == '"',
			s[0] == '`' && s[len(s)-1] == '`',
			s[0] == '\'' && s[len(s)-1] == '\'':
			return s[1 : len(s)-1]
		}
	}
	return s
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
