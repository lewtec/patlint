package js

import (
	"fmt"
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingest/ecma"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

// scriptRegion is one re-parseable ECMA body inside a host file (or the whole
// file for plain JS/TS). Offsets are host-file byte positions.
type scriptRegion struct {
	offset    uint32
	source    []byte
	grammar   string // "javascript" or "typescript"
	bodyStart uint32 // host start of body (inclusive)
	bodyEnd   uint32 // host end of body (exclusive)
}

func isSFCExt(policy ingest.PackQueries, path string) bool {
	return ingest.PathHasEmbeds(policy, path)
}

func isECMAMoveLanguage(claims []project.FamilyClaim, lang string) bool {
	return project.FamilyIDForLanguage(claims, lang) == ecma.FamilyID
}

// scriptRegions returns ECMA bodies for move/hygiene. Plain JS/TS files yield
// one region covering the whole file.
func scriptRegions(policy ingest.PackQueries, host []byte, fileRel string) []scriptRegion {
	lang, ok, err := ingest.AttributeHost(policy, fileRel)
	if err != nil || !ok {
		return nil
	}
	if err := ingestutil.RequireGrammar(ccgo.Engine{}, lang); err != nil {
		return nil
	}
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, host, fileRel, lang)
	if err != nil || pf == nil || pf.Root == nil {
		if pf != nil {
			pf.Close()
		}
		return nil
	}
	defer pf.Close()
	embs := ingest.EmbedRegions(policy, fileRel, host, pf.Root)
	if len(embs) > 0 {
		out := make([]scriptRegion, 0, len(embs))
		for _, e := range embs {
			out = append(out, scriptRegion{
				offset:    e.Offset,
				source:    e.Source,
				grammar:   e.Language,
				bodyStart: e.Offset,
				bodyEnd:   e.Offset + uint32(len(e.Source)),
			})
		}
		return out
	}
	return []scriptRegion{{
		offset:    0,
		source:    host,
		grammar:   lang,
		bodyStart: 0,
		bodyEnd:   uint32(len(host)),
	}}
}

func regionContaining(regions []scriptRegion, hostByte uint32) (scriptRegion, bool) {
	for _, r := range regions {
		if hostByte >= r.bodyStart && hostByte < r.bodyEnd {
			return r, true
		}
	}
	// Name at exact end of empty-ish regions — allow bodyEnd inclusive for start.
	for _, r := range regions {
		if hostByte >= r.bodyStart && hostByte <= r.bodyEnd {
			return r, true
		}
	}
	return scriptRegion{}, false
}

// parseRegionRoot parses a script region and returns a ParsedFile (caller Close).
func parseRegionRoot(r scriptRegion, fileHint string) (*ingestutil.ParsedFile, error) {
	g := r.grammar
	if g == "" {
		return nil, fmt.Errorf("%w: empty embed language", ErrUnknownGrammar)
	}
	return ingestutil.ParseSource(ccgo.Engine{}, r.source, fileHint, g)
}

// emptySFCShell builds a minimal host file that can hold moved ECMA decls.
// Frontmatter vs <script> is whichever shell the pack embed regions accept.
func emptySFCShell(policy ingest.PackQueries, fileRel, declText string) string {
	body := strings.TrimSpace(declText)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	if !isSFCExt(policy, fileRel) {
		return body
	}
	fm := "---\n" + body + "---\n"
	if shellHasPackEmbed(policy, fileRel, fm) {
		return fm
	}
	return "<script>\n" + body + "</script>\n"
}

func shellHasPackEmbed(policy ingest.PackQueries, fileRel, source string) bool {
	lang, ok, err := ingest.AttributeHost(policy, fileRel)
	if err != nil || !ok {
		return false
	}
	if err := ingestutil.RequireGrammar(ccgo.Engine{}, lang); err != nil {
		return false
	}
	src := []byte(source)
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, src, fileRel, lang)
	if err != nil || pf == nil {
		return false
	}
	defer pf.Close()
	return len(ingest.EmbedRegions(policy, fileRel, src, pf.Root)) > 0
}

// insertPosInRegion returns host byte offset for appending after the last import
// in the region (or at bodyStart if none).
func insertPosInRegion(r scriptRegion) uint32 {
	hint := r.grammar
	if hint != "" {
		hint = "embed." + hint
	}
	pf, err := parseRegionRoot(r, hint)
	if err != nil || pf == nil {
		return r.bodyStart
	}
	defer pf.Close()
	stmts := parseJSImportStatements(pf.Root, r.source)
	if len(stmts) == 0 {
		return r.bodyStart
	}
	last := stmts[len(stmts)-1]
	pos := last.endByte
	// Prefer after trailing newline inside the region.
	for pos < uint32(len(r.source)) && (r.source[pos] == '\n' || r.source[pos] == '\r') {
		pos++
		break
	}
	return r.offset + pos
}
