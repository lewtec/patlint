package ingest

import (
	"context"
	"sort"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/store"
)

// PackQueries is pack host/extract/embed. *pattern.LispVM implements it.
// Nil claims nothing. ingest cannot import LispVM while pattern imports ingest.
type PackQueries interface {
	AttributeHost(relPath string) (lang string, ok bool, err error)
	Extract(ctx context.Context, sess *project.Session, lang string, root *sitter.Node, source []byte, relPath string) (*project.FileExtract, error)
	PathHasEmbeds(relPath string) bool
	EmbedRegions(ctx context.Context, relPath string, source []byte, hostRoot *sitter.Node) []project.EmbeddedSource
	PathExtsForLanguage(lang string) []string
	Families() []project.FamilyClaim
	GrammarForLanguage(lang string) string
	ImportLine(lang string) string
	// ImportLineInfo is the compiled as-import template and its slots.
	ImportLineInfo(lang string) project.ImportLineClaim
	// PackageLineInfo is the compiled as-package token seq for lang.
	PackageLineInfo(lang string) project.PackageLineClaim
	// AtomicSpans is host (as-atomic TEXT) for lang.
	AtomicSpans(lang string) []string
	// DirectoryRepresentants is (under (path G…) (as-directory-representant)) globs.
	DirectoryRepresentants() []string
	// MatchDirectoryRepresentant is true when rel matches those globs.
	MatchDirectoryRepresentant(rel string) bool
	ResolveImport(spec string, ctx ImportResolveContext) string
	// ScopeNodeTypes is pack as-scope (node T) for lang (docs + abstract units).
	ScopeNodeTypes(lang string) []string
	// DocstringKind is pack (as-docstring KIND) for lang, or "".
	DocstringKind(lang string) string
	// ClassifyLeaf is pack as-keyword/as-ident/… type or token → highlight class.
	ClassifyLeaf(lang, grammarType string) string
	// ImportNeedFromRef is pack as-family: provider == lang or family id.
	ImportNeedFromRef(lang, ref string) (ImportNeed, bool)
	// RulesForLanguage is compiled as-family knobs for lang.
	RulesForLanguage(lang string) project.LanguageRules
	// FamilyForLanguage is the as-family id for lang, or "".
	FamilyForLanguage(lang string) string
	// LanguageInFamily is true when lang is a surface in family.
	LanguageInFamily(lang, family string) bool
	// LanguagesInFamily is host langs claimed for family.
	LanguagesInFamily(family string) []string
	// Layout is host (as-layout NAME…) section order for lang, or nil.
	Layout(lang string) []string
}

func AttributeHost(policy PackQueries, relPath string) (lang string, ok bool, err error) {
	if policy == nil {
		return "", false, nil
	}
	return policy.AttributeHost(relPath)
}

func attributeHost(policy PackQueries, relPath string) (lang string, ok bool, err error) {
	return AttributeHost(policy, relPath)
}

func ExtractFile(ctx context.Context, policy PackQueries, sess *project.Session, lang string, root *sitter.Node, source []byte, relPath string) (*project.FileExtract, error) {
	if policy == nil {
		return nil, ErrNilPolicy
	}
	return policy.Extract(ctx, sess, lang, root, source, relPath)
}

func extractFile(ctx context.Context, policy PackQueries, sess *project.Session, lang string, root *sitter.Node, source []byte, relPath string) (*project.FileExtract, error) {
	return ExtractFile(ctx, policy, sess, lang, root, source, relPath)
}

type extractInto interface {
	ExtractInto(ctx context.Context, sess *project.Session, st *store.Store, lang string, root *sitter.Node, source []byte, relPath string) error
}

func extractIntoStore(ctx context.Context, policy PackQueries, sess *project.Session, st *store.Store, lang string, root *sitter.Node, source []byte, relPath string) error {
	if x, ok := policy.(extractInto); ok {
		if err := x.ExtractInto(ctx, sess, st, lang, root, source, relPath); err != nil {
			return err
		}
		writeLangFlags(st, policy, lang)
		return nil
	}
	fe, err := ExtractFile(ctx, policy, sess, lang, root, source, relPath)
	if err != nil || fe == nil {
		return err
	}
	Ingest(st, []*project.FileExtract{fe}, policy)
	return nil
}

func languageRules(policy PackQueries, lang string) project.LanguageRules {
	if policy == nil {
		return project.LanguageRules{}
	}
	return policy.RulesForLanguage(lang)
}

func familiesOf(policy PackQueries) []project.FamilyClaim {
	if policy == nil {
		return nil
	}
	return policy.Families()
}

// GrammarID is the tree-sitter grammar for lang (as-grammar), or lang.
func GrammarID(policy PackQueries, lang string) string {
	if policy == nil || lang == "" {
		return lang
	}
	if g := policy.GrammarForLanguage(lang); g != "" {
		return g
	}
	return lang
}

// ImportLineTemplate is the host (as-import TEMPLATE) for lang, or "".
func ImportLineTemplate(policy PackQueries, lang string) string {
	return emitImportTokens(importInfo(policy, lang), "", "", "")
}

func importInfo(policy PackQueries, lang string) project.ImportLineClaim {
	if policy == nil || lang == "" {
		return project.ImportLineClaim{}
	}
	return policy.ImportLineInfo(lang)
}

func packageInfo(policy PackQueries, lang string) project.PackageLineClaim {
	if policy == nil || lang == "" {
		return project.PackageLineClaim{}
	}
	return policy.PackageLineInfo(lang)
}

func PathHasEmbeds(policy PackQueries, relPath string) bool {
	if policy == nil {
		return false
	}
	return policy.PathHasEmbeds(relPath)
}

func pathHasEmbeds(policy PackQueries, relPath string) bool {
	return PathHasEmbeds(policy, relPath)
}

func EmbedRegions(ctx context.Context, policy PackQueries, relPath string, source []byte, hostRoot *sitter.Node) []project.EmbeddedSource {
	if policy == nil {
		return nil
	}
	return policy.EmbedRegions(ctx, relPath, source, hostRoot)
}

func embedRegions(ctx context.Context, policy PackQueries, relPath string, source []byte, hostRoot *sitter.Node) []project.EmbeddedSource {
	return EmbedRegions(ctx, policy, relPath, source, hostRoot)
}

// PathExtsForFamily is the union of PathExtsForLanguage for languages in family.
func PathExtsForFamily(policy PackQueries, family string) []string {
	seen := map[string]struct{}{}
	var out []string
	if policy == nil {
		return nil
	}
	for _, lang := range policy.LanguagesInFamily(family) {
		for _, ext := range policy.PathExtsForLanguage(lang) {
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
