// Package script loads and runs .rft directive scripts (SPEC.md Pattern algebra §16).
//
// Lower-level model: match / under / rewrite / take / slot.
// (rule …) is only lint reporting metadata around a body — not the engine core.
package script

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/lewkit/x/text/report"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/datalog"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/projectfs"
	"github.com/lewtec/patlint/pkg/store"
)

// Report is optional lint metadata (from wrapping rule).
type Report struct {
	ID      string
	Level   report.Level
	Message string
}

// Action is one runnable step: match sites, optionally emit edits.
// Report != nil → contribute lint findings (text/SARIF).
// Emit != nil → produce edits for --fix.
type Action struct {
	Report  *Report
	Lang    string // empty = all languages
	Matcher *pattern.CompiledMatcher
	Take    string // capture name for edit locus; empty = whole match
	Emit    any    // nil = report-only; string | []any emit tree
	// Builtin non-empty for engine hooks (e.g. dead-imports).
	Builtin string
	Source  string // script path for errors
	Index   int    // step index in script
}

// Program is a loaded script.
type Program struct {
	Path    string
	Actions []Action
	// packSrcs are extract/rewrite lisp sources to merge into the Walker VM.
	packSrcs []pattern.Source
	// plan is the multi-leaf spine (lazy via EnsurePlan).
	plan *Plan
}

// Result is the outcome of Run.
type Result struct {
	Findings   []report.Finding
	ApplyEdits []project.Edit
	Actions    []Action
}

// Options control a script run.
type Options struct {
	Paths      []string
	LangFilter string
	// OnFinding is called as each finding is produced. A non-nil error stops the run.
	OnFinding func(report.Finding) error
	// FS is an optional project reader (e.g. overlay with dirty buffer text).
	// Nil means the real filesystem.
	FS projectfs.FS
}

// LoadFile reads and compiles a .rft script.
func LoadFile(path string) (*Program, error) {
	data, err := readOSPath(path)
	if err != nil {
		return nil, err
	}
	return Load(path, string(data))
}

func readOSPath(path string) ([]byte, error) {
	root, err := lewpath.Open(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return lewpath.New(filepath.Base(path)).ReadFile(root)
}

// Load compiles script source via LispVM.New, then decodes rewrite/rule/builtin.
func Load(path, src string) (*Program, error) {
	vm, err := pattern.New(prelude.FS, pattern.FromString(path, src))
	if err != nil {
		return nil, err
	}
	prog, err := decodeVM(path, vm)
	if err != nil {
		return nil, err
	}
	prog.packSrcs = []pattern.Source{pattern.FromString(path, src)}
	return prog, nil
}

const preludeRulesFile = "rules_rft.rft"

// mergePreludeRules appends always-on prelude rule actions not already in prog.
func mergePreludeRules(prog *Program) (*Program, error) {
	if prog == nil {
		prog = &Program{Path: "<builtin>"}
	}
	b, err := fs.ReadFile(prelude.FS, preludeRulesFile)
	if err != nil {
		return nil, fmt.Errorf("prelude rules: %w", err)
	}
	extra, err := Load(preludeRulesFile, string(b))
	if err != nil {
		return nil, fmt.Errorf("prelude rules: %w", err)
	}
	if extra == nil {
		return prog, nil
	}
	have := make(map[string]bool, len(prog.Actions))
	for _, a := range prog.Actions {
		if a.Report != nil {
			have[a.Report.ID] = true
		}
	}
	var add []Action
	for _, a := range extra.Actions {
		if a.Report != nil && have[a.Report.ID] {
			continue
		}
		add = append(add, a)
	}
	if len(add) == 0 {
		return prog, nil
	}
	prog.Actions = append(prog.Actions, add...)
	prog.plan = CompilePlan(prog)
	return prog, nil
}

func decodeVM(path string, vm *pattern.LispVM) (*Program, error) {
	if vm == nil {
		return nil, fmt.Errorf("%s: nil lispvm", path)
	}
	var actions []Action
	for _, f := range vm.ExtraForms() {
		act, ok, err := decodeForm(path, f)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		actions = append(actions, act)
	}
	prog := &Program{Path: path, Actions: actions}
	prog.plan = CompilePlan(prog)
	return prog, nil
}

func decodeForm(path string, f pattern.ExtraForm) (Action, bool, error) {
	src := f.File
	if src == "" {
		src = path
	}
	i := f.Index
	switch f.Head {
	case "def":
		return Action{}, false, nil
	case "rule":
		list, ok := f.Data.([]any)
		if !ok || len(list) != 5 {
			return Action{}, false, fmt.Errorf("%s: form %d: rule wants id level message body", src, i)
		}
		id, err := asString(list[1])
		if err != nil {
			return Action{}, false, fmt.Errorf("%s: form %d: rule id: %w", src, i, err)
		}
		level, err := asString(list[2])
		if err != nil {
			return Action{}, false, fmt.Errorf("%s: form %d: rule level: %w", src, i, err)
		}
		msg, err := asString(list[3])
		if err != nil {
			return Action{}, false, fmt.Errorf("%s: form %d: rule message: %w", src, i, err)
		}
		act, err := compileBody(list[4])
		if err != nil {
			return Action{}, false, fmt.Errorf("%s: form %d (rule %s): %w", src, i, id, err)
		}
		lvl, err := report.ParseLevel(level)
		if err != nil {
			return Action{}, false, fmt.Errorf("%s: form %d: rule level: %w", src, i, err)
		}
		act.Report = &Report{ID: id, Level: lvl, Message: msg}
		act.Source = src
		act.Index = i
		return act, true, nil
	case "rewrite":
		act, err := compileBody(f.Data)
		if err != nil {
			return Action{}, false, fmt.Errorf("%s: form %d: %w", src, i, err)
		}
		if act.Emit == nil {
			return Action{}, false, fmt.Errorf("%s: form %d: top-level rewrite needs emit", src, i)
		}
		act.Source = src
		act.Index = i
		return act, true, nil
	case "builtin":
		list, ok := f.Data.([]any)
		if !ok || len(list) != 5 {
			return Action{}, false, fmt.Errorf("%s: form %d: builtin wants id level name message", src, i)
		}
		id, _ := asString(list[1])
		level, _ := asString(list[2])
		name, _ := asString(list[3])
		msg, _ := asString(list[4])
		lvl, err := report.ParseLevel(level)
		if err != nil {
			return Action{}, false, fmt.Errorf("%s: form %d: builtin level: %w", src, i, err)
		}
		return Action{
			Report:  &Report{ID: id, Level: lvl, Message: msg},
			Builtin: name,
			Source:  src,
			Index:   i,
		}, true, nil
	case "progn":
		if f.Data == nil {
			return Action{}, false, nil
		}
		act, err := compileBody(f.Data)
		if err != nil {
			return Action{}, false, fmt.Errorf("%s: form %d: %w", src, i, err)
		}
		act.Source = src
		act.Index = i
		return act, true, nil
	default:
		act, err := compileBody(f.Data)
		if err != nil {
			return Action{}, false, fmt.Errorf("%s: form %d: %w", src, i, err)
		}
		act.Source = src
		act.Index = i
		return act, true, nil
	}
}

// compileBody peels rewrite/take/lang from the tree (including under nests)
// and compiles the remaining match tree.
func compileBody(v any) (Action, error) {
	var act Action
	matchTree, err := extractMatch(v, &act)
	if err != nil {
		return act, err
	}
	matchTree = normalizePathUnder(matchTree)
	m, err := pattern.MatcherFromExpanded(matchTree)
	if err != nil {
		return act, err
	}
	cm, err := pattern.CompileMatcher(m)
	if err != nil {
		return act, err
	}
	act.Matcher = cm
	return act, nil
}

// extractMatch walks the body tree, recording rewrite/take/lang on act,
// returning a pure match tree (under/path/Core only).
func extractMatch(v any, act *Action) (any, error) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return v, nil
	}
	head, _ := list[0].(string)
	switch head {
	case "rule", "builtin", "def":
		// Script packaging only at top level (Load). Nested → clear error, not
		// "unknown head" from the matcher compiler.
		return nil, fmt.Errorf("%s only allowed at script top level (not nested in match/rewrite bodies)", head)
	case "rewrite":
		if len(list) != 3 {
			return nil, fmt.Errorf("rewrite wants match and emit")
		}
		if act.Emit != nil {
			return nil, fmt.Errorf("nested rewrite")
		}
		act.Emit = list[2]
		return extractMatch(list[1], act)
	case "take":
		if len(list) != 3 {
			return nil, fmt.Errorf("take wants name and match")
		}
		name, err := asString(list[1])
		if err != nil {
			return nil, err
		}
		if act.Take != "" {
			return nil, fmt.Errorf("nested take")
		}
		act.Take = name
		return extractMatch(list[2], act)
	case "lang":
		if len(list) != 3 {
			return nil, fmt.Errorf("lang wants id and body")
		}
		lang, err := asString(list[1])
		if err != nil {
			return nil, err
		}
		if act.Lang != "" && act.Lang != lang {
			return nil, fmt.Errorf("conflicting lang")
		}
		act.Lang = lang
		return extractMatch(list[2], act)
	case "under":
		if len(list) < 3 {
			return nil, fmt.Errorf("under needs scope and body")
		}
		scope, body := list[1], list[2]
		if sl, ok := scope.([]any); ok && len(sl) >= 2 {
			if sh, _ := sl[0].(string); sh == "lang" {
				lang, err := asString(sl[1])
				if err != nil {
					return nil, err
				}
				if act.Lang != "" && act.Lang != lang {
					return nil, fmt.Errorf("conflicting lang")
				}
				act.Lang = lang
				return extractMatch(body, act)
			}
		}
		body2, err := extractMatch(body, act)
		if err != nil {
			return nil, err
		}
		return []any{"under", scope, body2}, nil
	case "path":
		if len(list) <= 2 {
			return v, nil
		}
		// (path glob body…)
		glob := list[1]
		var body any
		if len(list) == 3 {
			body = list[2]
		} else {
			body = append([]any{"and"}, list[2:]...)
		}
		body2, err := extractMatch(body, act)
		if err != nil {
			return nil, err
		}
		return []any{"path", glob, body2}, nil
	default:
		return v, nil
	}
}

// normalizePathUnder rewrites (under (path GLOB) BODY) → (path GLOB BODY) recursively.
func normalizePathUnder(v any) any {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return v
	}
	head, _ := list[0].(string)
	if head == "under" && len(list) >= 3 {
		scope, ok := list[1].([]any)
		if ok && len(scope) == 2 {
			if sh, _ := scope[0].(string); sh == "path" {
				body := normalizePathUnder(list[2])
				return []any{"path", scope[1], body}
			}
		}
		// normalize nested body
		out := make([]any, len(list))
		copy(out, list)
		out[2] = normalizePathUnder(list[2])
		return out
	}
	if head == "path" && len(list) >= 3 {
		out := make([]any, len(list))
		copy(out, list)
		out[2] = normalizePathUnder(list[2])
		return out
	}
	return v
}

func asString(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("want string, got %T", v)
	}
	return s, nil
}

// Run executes a program on sess using the multi-leaf spine plan when possible.
// Sites are collected per file (multi groups + residual matchers), then handlers
// (report / rewrite emit) run in script action order so claim/conflict stays stable.
func Run(ctx context.Context, sess *project.Session, prog *Program, opts Options) (Result, error) {
	var out Result
	out.Actions = prog.Actions
	if sess == nil {
		return out, ingest.ErrNilSession
	}
	rootAbs := sess.Root
	if rootAbs == "" {
		var err error
		rootAbs, err = filepath.Abs(".")
		if err != nil {
			return out, err
		}
	}

	plan := prog.EnsurePlan()
	claimed := map[string][]report.Span{}
	var apply []project.Edit

	fsys := opts.FS
	if fsys == nil {
		fsys = projectfs.OS{}
	}

	vm, err := pattern.New(prelude.FS, prog.packSrcs...)
	if err != nil {
		return out, err
	}
	walker, err := walker.NewWalker(ctx, sess, vm)
	if err != nil {
		return out, err
	}
	st := store.New()
	err = walker.WalkStore(ctx, opts.Paths, fsys, st, func(rel string) error {
		rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
		lang := ingest.FileLang(st, rel)
		if opts.LangFilter != "" && lang != opts.LangFilter && !vm.LanguageInFamily(lang, opts.LangFilter) {
			return nil
		}

		abs := lewpath.New(rootAbs, filepath.FromSlash(rel)).String()
		source, err := fsys.ReadFile(abs)
		if err != nil {
			return err
		}
		pf, err := ingestutil.ParseSource(ctx, sess.Engine(), source, abs, ingest.GrammarID(vm, lang))
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
		defer pf.Close()

		var fileResult *project.Result
		if plan.NeedLinks {
			var evalErr error
			fileResult, evalErr = ingest.EvalStore(ctx, rootAbs, st, vm)
			if evalErr != nil {
				return evalErr
			}
		}

		// action index → sites (precomputed on spine / residual)
		sites := map[int][]pattern.Match{}
		pol := vm.TapePolicy(rel)

		for _, g := range plan.Groups {
			if g.Lang != "" && g.Lang != lang {
				continue
			}
			if !acceptsFileKey(g.FileKey, rel) {
				continue
			}
			tagged, err := pattern.MatchFileMultiLeavesTape(sess, rootAbs, rel, source, pf.Root, g.Multi, g.Arms, fileResult, pol)
			if err != nil {
				return fmt.Errorf("spine multi: %w", err)
			}
			for _, tm := range tagged {
				sites[tm.ID] = append(sites[tm.ID], tm.Match)
			}
		}
		// under: region once, then body multi on each domain
		for _, ug := range plan.Unders {
			if ug.Lang != "" && ug.Lang != lang {
				continue
			}
			if !acceptsFileKey(ug.FileKey, rel) {
				continue
			}
			if ug.Region == nil || !ug.Region.AcceptsFile(rel) {
				continue
			}
			regs, err := pattern.MatchFileMatcherPolicy(ctx, sess, rootAbs, rel, source, pf.Root, ug.Region, fileResult, pol)
			if err != nil {
				return fmt.Errorf("spine under region: %w", err)
			}
			for _, reg := range regs {
				tagged, err := pattern.MatchFileMultiLeavesDomain(
					sess, rootAbs, rel, source, pf.Root, ug.Multi, ug.Arms, reg.Span, fileResult, pol)
				if err != nil {
					return fmt.Errorf("spine under body: %w", err)
				}
				for _, tm := range tagged {
					m := pattern.MergeRegionCaptures(reg, tm.Match)
					sites[tm.ID] = append(sites[tm.ID], m)
				}
			}
		}
		for _, ai := range plan.Residual {
			act := prog.Actions[ai]
			if act.Lang != "" && act.Lang != lang {
				continue
			}
			if act.Matcher == nil || !act.Matcher.AcceptsFile(rel) {
				continue
			}
			ms, err := pattern.MatchFileMatcherPolicy(ctx, sess, rootAbs, rel, source, pf.Root, act.Matcher, fileResult, pol)
			if err != nil {
				return fmt.Errorf("%s step %d: match %s: %w", act.Source, act.Index, rel, err)
			}
			sites[ai] = ms
		}

		// Handlers in script order (rules + rewrites share sites).
		for i, act := range prog.Actions {
			if act.Builtin != "" {
				if err := runBuiltin(ctx, walker.VM, act, rootAbs, rel, store.ProjectExtract(st, rel), source, claimed, &apply, &out, opts); err != nil {
					return err
				}
				continue
			}
			if act.Lang != "" && act.Lang != lang {
				continue
			}
			if act.Report != nil {
				continue
			}
			for _, m := range sites[i] {
				if err := handleSite(act, m, source, rel, claimed, &apply, &out, opts); err != nil {
					return err
				}
			}
		}
		if err := datalog.Eval(ctx, st, prog.RuleClauses(rel), &ruleHost{prog: prog, sites: sites}); err != nil {
			return err
		}
		if err := findingsFromStore(prog, st, rel, source, sites, claimed, &apply, &out, opts); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	out.ApplyEdits = apply
	return out, nil
}

func findingsFromStore(prog *Program, st *store.Store, rel string, source []byte, sites map[int][]pattern.Match, claimed map[string][]report.Span, apply *[]project.Edit, out *Result, opts Options) error {
	if st == nil || prog == nil {
		return nil
	}
	byID := map[string]int{}
	for i, act := range prog.Actions {
		if act.Report != nil && act.Builtin == "" {
			byID[act.Report.ID] = i
		}
	}
	for _, t := range st.Rows(store.RelationFinding) {
		if len(t) < 6 || strings.TrimPrefix(t[0], "./") != rel {
			continue
		}
		ai, ok := byID[t[3]]
		if !ok {
			continue
		}
		act := prog.Actions[ai]
		sp := ingestutil.Span{StartByte: store.Atoi(t[1]), EndByte: store.Atoi(t[2])}
		if sp.EndByte <= sp.StartByte {
			continue
		}
		var edits []project.Edit
		for _, m := range sites[ai] {
			ms := m.Span
			if act.Take != "" {
				if cap, ok := m.CaptureFirst(act.Take); ok {
					ms = cap
				}
			}
			if ms.StartByte != sp.StartByte || ms.EndByte != sp.EndByte {
				continue
			}
			ed, err := editsForMatch(act, m, source)
			if err != nil {
				return err
			}
			edits = ed
			break
		}
		line, col, endLine, endCol, snippet, err := report.SpanLoc(source, reportSpan(sp))
		if err != nil {
			return err
		}
		shown := reportEdits(edits)
		skip := report.EditsOverlap(shown, claimed[rel])
		f := report.Finding{
			RuleID:     t[3],
			Level:      report.Level(t[4]),
			Message:    t[5],
			File:       rel,
			Line:       line,
			Column:     col,
			EndLine:    endLine,
			EndCol:     endCol,
			Snippet:    snippet,
			Edits:      shown,
			Fixable:    len(edits) > 0,
			FixSkipped: skip,
			Source:     source,
		}
		if opts.OnFinding != nil {
			if err := opts.OnFinding(f); err != nil {
				return err
			}
		}
		out.Findings = append(out.Findings, f)
		if len(edits) > 0 && !skip {
			claimed[rel] = append(claimed[rel], bodySpans(edits)...)
			*apply = append(*apply, edits...)
		}
	}
	return nil
}

// handleSite applies report/emit handlers for one match site.
func handleSite(act Action, m pattern.Match, source []byte, rel string, claimed map[string][]report.Span, apply *[]project.Edit, out *Result, opts Options) error {
	edits, err := editsForMatch(act, m, source)
	if err != nil {
		return fmt.Errorf("%s step %d: emit %s: %w", act.Source, act.Index, rel, err)
	}
	// skip empty multi take
	if act.Take != "" && len(edits) == 0 && act.Emit != nil {
		return nil
	}
	if act.Report == nil && act.Emit == nil {
		return nil
	}
	if act.Report == nil && act.Emit != nil {
		if len(edits) > 0 && !report.EditsOverlap(reportEdits(edits), claimed[rel]) {
			claimed[rel] = append(claimed[rel], bodySpans(edits)...)
			*apply = append(*apply, edits...)
		}
		return nil
	}

	span := m.Span
	if act.Take != "" {
		if sp, ok := m.CaptureFirst(act.Take); ok {
			span = sp
		} else {
			return nil
		}
	}
	line, col, endLine, endCol, snippet, err := report.SpanLoc(source, reportSpan(span))
	if err != nil {
		return err
	}
	shown := reportEdits(edits)
	skip := report.EditsOverlap(shown, claimed[rel])
	f := report.Finding{
		RuleID:     act.Report.ID,
		Level:      act.Report.Level,
		Message:    act.Report.Message,
		File:       rel,
		Line:       line,
		Column:     col,
		EndLine:    endLine,
		EndCol:     endCol,
		Snippet:    snippet,
		Edits:      shown,
		Fixable:    len(edits) > 0,
		FixSkipped: skip,
		Source:     source,
	}
	if opts.OnFinding != nil {
		if err := opts.OnFinding(f); err != nil {
			return err
		}
	}
	out.Findings = append(out.Findings, f)
	if len(edits) > 0 && !skip {
		claimed[rel] = append(claimed[rel], bodySpans(edits)...)
		*apply = append(*apply, edits...)
	}
	return nil
}

func editsForMatch(act Action, m pattern.Match, source []byte) ([]project.Edit, error) {
	if act.Emit == nil {
		return nil, nil
	}
	if act.Take != "" {
		spans := m.Captures[act.Take]
		if len(spans) == 0 {
			return nil, nil
		}
		var edits []project.Edit
		for _, sp := range spans {
			text, err := pattern.InstantiateEmit(act.Emit, source, sp, m)
			if err != nil {
				return nil, err
			}
			edits = append(edits, project.Edit{File: m.File, Span: sp, NewText: text})
		}
		return edits, nil
	}
	text, err := pattern.InstantiateEmit(act.Emit, source, m.Span, m)
	if err != nil {
		return nil, err
	}
	return []project.Edit{{File: m.File, Span: m.Span, NewText: text}}, nil
}

func runBuiltin(ctx context.Context, policy ingest.PackQueries, act Action, root, rel string, fe *project.FileExtract, source []byte, claimed map[string][]report.Span, apply *[]project.Edit, out *Result, opts Options) error {
	if act.Builtin != "dead-imports" {
		return fmt.Errorf("unknown builtin %q", act.Builtin)
	}
	if fe == nil {
		return nil
	}
	st, err := ingest.FileEffects(ctx, root, []*project.FileExtract{fe}, policy)
	if err != nil {
		return err
	}
	editsAt := map[string]project.Edit{}
	for _, t := range st.Rows(store.RelationEdit) {
		if len(t) < 4 {
			continue
		}
		ed := project.Edit{
			File:    rel,
			Span:    ingestutil.Span{StartByte: store.Atoi(t[1]), EndByte: store.Atoi(t[2])},
			NewText: t[3],
		}
		editsAt[t[1]+"\x00"+t[2]] = ed
	}
	for _, t := range st.Rows(store.RelationFinding) {
		if len(t) < 6 || t[3] != datalog.DeadImportID {
			continue
		}
		sp := ingestutil.Span{StartByte: store.Atoi(t[1]), EndByte: store.Atoi(t[2])}
		if sp.EndByte <= sp.StartByte {
			continue
		}
		line, col, endLine, endCol, snippet, err := report.SpanLoc(source, reportSpan(sp))
		if err != nil {
			return err
		}
		id, level, msg := t[3], report.Level(t[4]), t[5]
		if act.Report != nil {
			id, level, msg = act.Report.ID, act.Report.Level, act.Report.Message
		}
		ed, has := editsAt[t[1]+"\x00"+t[2]]
		var site []report.Edit
		if has {
			site = reportEdits([]project.Edit{ed})
		}
		skip := report.EditsOverlap(site, claimed[rel])
		f := report.Finding{
			RuleID: id, Level: level, Message: msg,
			File: rel, Line: line, Column: col, EndLine: endLine, EndCol: endCol,
			Snippet: snippet, Edits: site, Fixable: has, FixSkipped: skip, Source: source,
		}
		if opts.OnFinding != nil {
			if err := opts.OnFinding(f); err != nil {
				return err
			}
		}
		out.Findings = append(out.Findings, f)
		if has && !skip {
			claimed[rel] = append(claimed[rel], report.Span{StartByte: ed.StartByte, EndByte: ed.EndByte})
			*apply = append(*apply, ed)
		}
	}
	return nil
}

func reportSpan(sp ingestutil.Span) report.Span {
	return report.Span{StartByte: sp.StartByte, EndByte: sp.EndByte}
}

func reportEdits(edits []project.Edit) []report.Edit {
	if len(edits) == 0 {
		return nil
	}
	out := make([]report.Edit, len(edits))
	for i, edit := range edits {
		out[i] = report.Edit{
			File:      edit.File,
			StartByte: edit.StartByte,
			EndByte:   edit.EndByte,
			NewText:   edit.NewText,
		}
	}
	return out
}

func bodySpans(edits []project.Edit) []report.Span {
	out := make([]report.Span, 0, len(edits))
	for _, edit := range edits {
		if edit.Empty() {
			continue
		}
		out = append(out, report.Span{StartByte: edit.StartByte, EndByte: edit.EndByte})
	}
	return out
}

// ReportRules collects SARIF reporting descriptors from script actions and findings.
func ReportRules(res Result) []report.Rule {
	seen := map[string]report.Rule{}
	order := []string{}
	add := func(id, msg string, level report.Level) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = report.Rule{ID: id, Message: msg, Level: level}
		order = append(order, id)
	}
	for _, a := range res.Actions {
		if a.Report == nil {
			continue
		}
		add(a.Report.ID, a.Report.Message, a.Report.Level)
	}
	for _, f := range res.Findings {
		add(f.RuleID, f.Message, f.Level)
	}
	out := make([]report.Rule, 0, len(order))
	for _, id := range order {
		out = append(out, seen[id])
	}
	return out
}
