package pattern

import (
	"context"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
)

// PaintClasses runs program paint actions on the host tree and every embed
// region for relPath. Embed actions reparse their region; host-language paint
// (and global empty-path paint) also runs on each embed tree so keywords/strings
// inside islands are not skipped (SPEC.md Regions).
// Later actions override earlier on the same span.
func PaintClasses(ctx context.Context, p *ExtractProgram, sess *project.Session, root *sitter.Node, source []byte, relPath string) map[[2]uint32]string {
	out := map[[2]uint32]string{}
	if p == nil || root == nil {
		return out
	}
	rel := stringsTrimDotSlash(filepathToSlash(relPath))

	for _, act := range p.Actions {
		if act.Kind != ExtractPaint || act.Matcher == nil || act.TokenClass == "" {
			continue
		}
		if !actionAcceptsPath(act.Paths, rel) {
			continue
		}
		var ms []Match
		var err error
		if act.Embed {
			ms, err = matchEmbedAction(ctx, sess, relPath, source, root, act, nil, p)
		} else {
			ms, err = MatchFileMatcherPolicy(ctx, sess, ".", relPath, source, root, act.Matcher, nil, p.tapePolicy(p.hostLangFor(rel)))
		}
		if err != nil {
			continue
		}
		applyPaintMatches(out, ms, act, source)
	}

	// Host packs for language L (path-scoped) and global paint must also run
	// on embed regions of language L even when the host file path is not L's glob.
	embs := uniqueEmbedRegions(ctx, sess, p, rel, relPath, source, root, nil)
	for _, emb := range embs {
		for _, act := range p.Actions {
			if act.Kind != ExtractPaint || act.Matcher == nil || act.TokenClass == "" {
				continue
			}
			if act.Embed {
				continue // already applied via matchEmbedAction
			}
			if !actionAppliesToLang(act, emb.lang) {
				continue
			}
			ms, err := MatchFileMatcherPolicy(ctx, sess, ".", relPath, emb.content, emb.root, act.Matcher, nil, p.tapePolicy(emb.lang))
			if err != nil {
				continue
			}
			for i := range ms {
				ms[i] = offsetMatch(ms[i], emb.base)
			}
			applyPaintMatches(out, ms, act, source)
		}
	}
	for _, emb := range embs {
		emb.close()
	}
	return out
}

func applyPaintMatches(out map[[2]uint32]string, ms []Match, act ExtractAction, source []byte) {
	for _, m := range ms {
		locus := m.Span
		if act.Take != "" {
			if sp, ok := m.CaptureFirst(act.Take); ok && !sp.Empty() {
				locus = sp
			} else {
				continue
			}
		}
		if locus.Empty() || int(locus.EndByte) > len(source) {
			continue
		}
		out[[2]uint32{locus.StartByte, locus.EndByte}] = act.TokenClass
	}
}

// actionAppliesToLang reports whether a host/global action should run on a
// tree of language lang (embed guest). Path globs do not apply.
func actionAppliesToLang(act ExtractAction, lang string) bool {
	if lang == "" {
		return false
	}
	if len(act.Paths) == 0 {
		return true // global highlight.rft
	}
	return act.HostLang == lang || act.Lang == lang
}

type embedLocus struct {
	lang    string
	base    uint32
	content []byte
	root    *sitter.Node
	// pf kept so root stays valid while painting; caller closes via close().
	pf *ingestutil.ParsedFile
}

func (e embedLocus) close() {
	if e.pf != nil {
		e.pf.Close()
	}
}

// uniqueEmbedRegions finds path-accepted embed claims, reparses each host span
// once per (span, lang). Caller must close returned loci.
func uniqueEmbedRegions(ctx context.Context, sess *project.Session, p *ExtractProgram, rel, relPath string, source []byte, hostRoot *sitter.Node, scratch *fileScratch) []embedLocus {
	if p == nil || hostRoot == nil {
		return nil
	}
	type key struct {
		s, e uint32
		lang string
	}
	seen := map[key]struct{}{}
	var out []embedLocus
	for _, act := range p.Actions {
		if !act.Embed || act.Region == nil || act.Lang == "" {
			continue
		}
		if !actionAcceptsPath(act.Paths, rel) {
			continue
		}
		hostPol := p.tapePolicy(p.hostLangFor(relPath))
		if scratch != nil {
			hostPol = scratch.pol
		}
		regs, err := matchFileMatcherPol(ctx, sess, ".", relPath, source, hostRoot, act.Region, nil, hostPol, scratch)
		if err != nil {
			continue
		}
		gid := act.Lang
		if p != nil {
			gid = p.grammarID(act.Lang)
		}
		if sess == nil || sess.Engine() == nil || !sess.Engine().Has(ctx, gid) {
			continue
		}
		for _, reg := range regs {
			if reg.Empty() || int(reg.EndByte) > len(source) || reg.StartByte >= reg.EndByte {
				continue
			}
			raw := source[reg.StartByte:reg.EndByte]
			content, base := stripEmbedDelims(raw, reg.StartByte)
			if len(content) == 0 {
				continue
			}
			k := key{base, base + uint32(len(content)), act.Lang}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			pf, err := ingestutil.ParseSource(ctx, sess.Engine(), content, relPath+"#"+act.Lang, gid)
			if err != nil || pf == nil || pf.Root == nil {
				if pf != nil {
					pf.Close()
				}
				continue
			}
			out = append(out, embedLocus{
				lang:    act.Lang,
				base:    base,
				content: content,
				root:    pf.Root,
				pf:      pf,
			})
		}
	}
	return out
}

func classForCell(paint map[[2]uint32]string, start, end uint32, leafType, lang string, p *ExtractProgram) string {
	if paint != nil {
		if cls, ok := paint[[2]uint32{start, end}]; ok {
			return cls
		}
		for k, cls := range paint {
			if k[0] <= start && end <= k[1] {
				return cls
			}
		}
	}
	if p != nil {
		return p.ClassifyLeaf(lang, leafType)
	}
	return ""
}

func stringsTrimDotSlash(rel string) string {
	for {
		switch {
		case len(rel) >= 2 && rel[0] == '.' && rel[1] == '/':
			rel = rel[2:]
		case len(rel) >= 1 && rel[0] == '/':
			rel = rel[1:]
		default:
			return rel
		}
	}
}
