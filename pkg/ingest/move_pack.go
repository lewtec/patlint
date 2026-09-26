package ingest

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// packMove is pack facts for dest-import / qualify apply.
// Policy is .rft (as-import seq, as-layout, as-family). This type only reads it.
type packMove struct {
	queries    PackQueries
	language   string
	importLine project.ImportLineClaim
	layout     []string
	rules      project.LanguageRules
}

func loadPackMove(queries PackQueries, language string) packMove {
	loaded := packMove{queries: queries, language: language}
	if queries == nil || language == "" {
		return loaded
	}
	loaded.importLine = queries.ImportLineInfo(language)
	loaded.layout = queries.Layout(language)
	loaded.rules = queries.RulesForLanguage(language)
	return loaded
}

func (p packMove) emitImportTokens(specifier, leaf, qualifier string) string {
	return ingestutil.EmitSeq(p.importLine.Tokens, map[string]string{
		"path": specifier, "leaf": leaf, "qual": qualifier,
	})
}

func (p packMove) emitImportLine(specifier, leaf, qualifier string) string {
	return emitImportLine(p.importLine, specifier, leaf, qualifier)
}

func (p packMove) importLineOkay() bool {
	return importLineOkay(p.importLine)
}

func emitImportTokens(importLine project.ImportLineClaim, specifier, leaf, qualifier string) string {
	return ingestutil.EmitSeq(importLine.Tokens, map[string]string{
		"path": specifier, "leaf": leaf, "qual": qualifier,
	})
}

func emitImportLine(importLine project.ImportLineClaim, specifier, leaf, qualifier string) string {
	s := strings.TrimLeft(emitImportTokens(importLine, specifier, leaf, qualifier), "\n")
	if s == "" {
		return ""
	}
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

func importLineOkay(importLine project.ImportLineClaim) bool {
	return len(importLine.Tokens) > 0
}

func (p packMove) sameMoveModule(a, b string) bool {
	a = strings.TrimPrefix(a, "./")
	b = strings.TrimPrefix(b, "./")
	if a == b {
		return true
	}
	if p.rules.PackageScopedBareNames {
		return importFileDir(a) == importFileDir(b)
	}
	return false
}
