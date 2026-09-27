package pattern

import (
	"context"
	"strconv"
	"strings"

	"github.com/lewtec/patlint/pkg/datalog"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/store"
	"github.com/lewtec/patlint/pkg/tape"
)

// IngestClauses is stratum-1 sugar: one Horn clause per as-* action.
// Body is the finder builtin $as(idx, …); head is the ingest relation.
func (p *ExtractProgram) IngestClauses(relPath string) datalog.Program {
	if p == nil {
		return nil
	}
	var out datalog.Program
	rel := strings.TrimPrefix(filepathToSlash(relPath), "./")
	for i, act := range p.Actions {
		if act.Matcher == nil || act.Kind == ExtractPaint || act.Embed {
			continue
		}
		if !actionAcceptsPath(act.Paths, rel) {
			continue
		}
		rel, vars := ingestHead(act)
		if rel == "" {
			continue
		}
		idx := strconv.Itoa(i)
		headArgs := append([]datalog.Arg{datalog.Const(relPath)}, vars...)
		bodyArgs := append([]datalog.Arg{datalog.Const(idx)}, vars...)
		out = append(out, datalog.Clause{
			Head: datalog.Lit{Rel: rel, Args: headArgs},
			Body: []datalog.Lit{{Rel: "$as", Args: bodyArgs}},
		})
	}
	return out
}

func ingestHead(act ExtractAction) (string, []datalog.Arg) {
	v := func(names ...string) []datalog.Arg {
		out := make([]datalog.Arg, len(names))
		for i, n := range names {
			out[i] = datalog.Var(n)
		}
		return out
	}
	switch act.Kind {
	case ExtractPackage:
		return store.RelationPackage, v("n", "e")
	case ExtractAtom:
		return store.RelationAtom, v("n", "s", "e", "x", "sc")
	case ExtractUse:
		return store.RelationUse, v("n", "s", "e", "scope", "si", "np", "id")
	case ExtractImport:
		return store.RelationImport, v("local", "path", "mem", "s", "e", "ts", "te", "al", "ps", "pe", "star")
	case ExtractReexport:
		return store.RelationReexport, v("ex", "sn", "path", "star", "ss", "se")
	case ExtractDefault:
		return store.RelationDefault, v("n")
	case ExtractScope:
		return store.RelationScope, v("i", "p", "s", "e", "hole")
	case ExtractFlow:
		return store.RelationFlow, v("s", "e", "class")
	default:
		return "", nil
	}
}

// ingestHost is the finder oracle for $as: matcher → ephemeral rows.
type ingestHost struct {
	ctx     context.Context
	st      *store.Store
	sess    *project.Session
	p       *ExtractProgram
	root    *sitter.Node
	source  []byte
	fp      string
	useID   *int
	err     error
	cache   map[string][][]string
	scratch *fileScratch
	pol     tape.Policy
}

func (h *ingestHost) Builtin(name string, args []string) [][]string {
	if name != "as" || len(args) < 1 || h == nil || h.p == nil {
		return nil
	}
	if h.cache != nil {
		if rows, ok := h.cache[args[0]]; ok {
			return rows
		}
	}
	i, err := strconv.Atoi(args[0])
	if err != nil || i < 0 || i >= len(h.p.Actions) {
		return nil
	}
	act := h.p.Actions[i]
	if act.Matcher == nil || act.Kind == ExtractPaint {
		return nil
	}
	ms, merr := matchFileMatcherPol(h.ctx, h.sess, ".", h.fp, h.source, h.root, act.Matcher, nil, h.pol, h.scratch)
	if merr != nil {
		h.err = merr
		return nil
	}
	tmp := store.New()
	applyExtractMatches(tmp, h.fp, h.useID, h.p, act, h.source, h.root, ms, 0, h.scratch.nodes(h.root))
	rel, _ := ingestHead(act)
	if rel == "" {
		return nil
	}
	var out [][]string
	if rel == store.RelationScope {
		n := 0
		for _, t := range h.st.Rows(store.RelationScope) {
			if len(t) > 0 && t[0] == h.fp {
				n++
			}
		}
		for _, t := range tmp.Rows(store.RelationScope) {
			if len(t) < 2 {
				continue
			}
			row := append([]string(nil), t[1:]...)
			row[0] = store.ItoaInt(n)
			n++
			out = append(out, row)
		}
	} else {
		for _, t := range tmp.Rows(rel) {
			if len(t) < 2 {
				continue
			}
			out = append(out, append([]string(nil), t[1:]...))
		}
	}
	for _, t := range tmp.Rows(store.RelationUseSegment) {
		h.st.Append(store.RelationUseSegment, t)
	}
	if h.cache == nil {
		h.cache = map[string][][]string{}
	}
	h.cache[args[0]] = out
	return out
}
