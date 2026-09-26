package script

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ignore"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/report"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
)

// TestFileName is the metadata file inside a fixture case.
const TestFileName = "test.toml"

// TestdataSubdir is the conventional cases directory under a pack root's .patlint.
const TestdataSubdir = "testdata"

const (
	KindRun     = "run"
	KindExtract = "extract"
	KindIngest  = "ingest"
	KindGrep    = "grep"
	KindRewrite = "rewrite"
	KindMv      = "mv"
)

// CaseMeta is the decoded test.toml.
// kind selects allowed scalars and row tables (singular names).
type CaseMeta struct {
	Kind        string `toml:"kind"`
	Description string `toml:"description,omitempty"`

	Fix *bool `toml:"fix,omitempty"`

	Finding []WantFinding `toml:"finding,omitempty"`
	File    []WantFile    `toml:"file,omitempty"`
	Atom    []WantAtom    `toml:"atom,omitempty"`
	Use     []WantUse     `toml:"use,omitempty"`
	Alias   []WantAlias   `toml:"alias,omitempty"`

	Lang             string `toml:"lang,omitempty"`
	Pattern          string `toml:"pattern,omitempty"`
	Emit             string `toml:"emit,omitempty"`
	ExpectMatchCount *int   `toml:"expect_match_count,omitempty"`

	Source      string `toml:"source"`
	Destination string `toml:"destination"`
	Error       string `toml:"error,omitempty"`
}

// WantFinding is a partial report.Finding assertion (TOML ground truth).
// Only set fields are compared; omitted fields are wildcards.
type WantFinding struct {
	ID      string `toml:"id"`
	Path    string `toml:"path"`
	File    string `toml:"file"` // alias for path
	Line    *int   `toml:"line"`
	Column  *int   `toml:"column"`
	EndLine *int   `toml:"end_line"`
	EndCol  *int   `toml:"end_col"`
	Message string `toml:"message"`
	Level   string `toml:"level"`
	Snippet string `toml:"snippet"`
}

// WantFile is one ingest [[file]] row.
type WantFile struct {
	Language string `toml:"language"`
	Path     string `toml:"path"`
}

// WantAtom is one [[atom]] row. Extract uses Name; ingest uses Reference+bytes.
type WantAtom struct {
	Name      string `toml:"name,omitempty"`
	Reference string `toml:"reference,omitempty"`
	StartByte uint32 `toml:"start_byte,omitempty"`
	EndByte   uint32 `toml:"end_byte,omitempty"`
}

// WantUse is one ingest [[use]] row.
type WantUse struct {
	Reference      string `toml:"reference"`
	StartByte      uint32 `toml:"start_byte"`
	EndByte        uint32 `toml:"end_byte"`
	Target         string `toml:"target"`
	ViaImportAlias bool   `toml:"via_import_alias,omitempty"`
}

// WantAlias is one ingest [[alias]] row.
type WantAlias struct {
	Reference string `toml:"reference"`
	StartByte uint32 `toml:"start_byte"`
	EndByte   uint32 `toml:"end_byte"`
	Target    string `toml:"target"`
}

// Case is one fixture directory under .patlint/testdata/<name>/.
type Case struct {
	Name     string // directory base name
	Dir      string // absolute case directory
	Meta     CaseMeta
	Scenario string // abs path to scenario/
	Expected string // abs path to expected/ (may not exist)
}

// Rel is Dir relative to base, slash-separated. Falls back to Dir.
func (c Case) Rel(base string) string {
	if base == "" {
		return filepath.ToSlash(c.Dir)
	}
	rel, err := filepath.Rel(base, c.Dir)
	if err != nil {
		return filepath.ToSlash(c.Dir)
	}
	return filepath.ToSlash(rel)
}

// DiscoverCases lists case dirs under packRoot/.patlint/testdata.
// packRoot is typically the repository root (ListPackScripts root).
func DiscoverCases(packRoot string) ([]Case, error) {
	root, err := filepath.Abs(packRoot)
	if err != nil {
		return nil, err
	}
	return DiscoverCasesIn(lewpath.New(root, PackSubdir, TestdataSubdir).String())
}

// DiscoverCasesIn lists immediate child dirs that contain test.toml.
// Missing dir → empty list. A child without test.toml is skipped.
func DiscoverCasesIn(dir string) ([]Case, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Case
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		child := lewpath.New(abs, e.Name()).String()
		if _, err := os.Stat(lewpath.New(child, TestFileName).String()); err != nil {
			continue
		}
		c, err := LoadCase(child)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, c)
	}
	return out, nil
}

// LoadCases loads path as one case (has test.toml) or as a suite of cases.
func LoadCases(ctx context.Context, path string) ([]Case, error) {
	var out []Case
	err := WalkCases(ctx, path, func(c Case) error {
		out = append(out, c)
		return nil
	})
	return out, err
}

// WalkCases visits one case or each suite child, in directory order.
func WalkCases(ctx context.Context, path string, fn func(Case) error) error {
	if fn == nil {
		return fmt.Errorf("WalkCases: nil fn")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return err
	}
	if _, err := os.Stat(lewpath.New(abs, TestFileName).String()); err == nil {
		c, err := LoadCase(abs)
		if err != nil {
			return err
		}
		return fn(c)
	}
	ents, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	n := 0
	for _, e := range ents {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !e.IsDir() {
			continue
		}
		child := lewpath.New(abs, e.Name()).String()
		if _, err := os.Stat(lewpath.New(child, TestFileName).String()); err != nil {
			continue
		}
		c, err := LoadCase(child)
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		n++
		if err := fn(c); err != nil {
			return err
		}
	}
	if n == 0 {
		return fmt.Errorf("%s: no test.toml cases", path)
	}
	return nil
}

// WalkCasesRecursive visits every test.toml under root.
// Skips gitignore and builtin dirs (node_modules, .git, …).
// Does not skip refactree-ignored / linguist-generated (testdata is
// refactree-ignored for product crawls, not for rft test).
// A case directory is not walked further (scenario/ and expected/ stay out).
// eng nil → Collect(root).
func WalkCasesRecursive(ctx context.Context, root string, eng *ignore.Engine, fn func(Case) error) error {
	if fn == nil {
		return fmt.Errorf("WalkCasesRecursive: nil fn")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return err
	}
	if eng == nil {
		eng, err = ignore.Collect(ctx, abs)
		if err != nil {
			return err
		}
	}
	n := 0
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != abs && eng.GitSkipDir(path) {
			return filepath.SkipDir
		}
		tomlPath := lewpath.New(path, TestFileName).String()
		if _, err := os.Stat(tomlPath); err != nil {
			return nil
		}
		if eng.GitHidden(tomlPath, false) {
			return filepath.SkipDir
		}
		c, err := LoadCase(path)
		if err != nil {
			return fmt.Errorf("%s: %w", tomlPath, err)
		}
		n++
		if err := fn(c); err != nil {
			return err
		}
		return filepath.SkipDir
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s: no test.toml cases", root)
	}
	return nil
}

// LoadCase reads test.toml and resolves scenario/ / expected/ under dir.
func LoadCase(dir string) (Case, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Case{}, err
	}
	metaPath := lewpath.New(abs, TestFileName).String()
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return Case{}, err
	}
	var meta CaseMeta
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&meta); err != nil {
		return Case{}, fmt.Errorf("parse %s: %w", TestFileName, err)
	}
	if err := meta.validate(); err != nil {
		return Case{}, fmt.Errorf("%s: %w", TestFileName, err)
	}
	scenario := lewpath.New(abs, "scenario").String()
	st, err := os.Stat(scenario)
	if err != nil || !st.IsDir() {
		return Case{}, fmt.Errorf("missing scenario/ directory")
	}
	expected := lewpath.New(abs, "expected").String()
	if st, err := os.Stat(expected); err != nil || !st.IsDir() {
		expected = ""
	}
	return Case{
		Name:     filepath.Base(abs),
		Dir:      abs,
		Meta:     meta,
		Scenario: scenario,
		Expected: expected,
	}, nil
}

func (m CaseMeta) validate() error {
	switch m.Kind {
	case KindRun:
		if err := m.rejectScalars("lang", m.Lang, "pattern", m.Pattern, "emit", m.Emit, "source", m.Source, "destination", m.Destination, "error", m.Error); err != nil {
			return err
		}
		if m.ExpectMatchCount != nil {
			return fmt.Errorf("run: unexpected expect_match_count")
		}
		if err := m.rejectTables(false, true, true, true); err != nil {
			return err
		}
		for i, f := range m.Finding {
			if strings.TrimSpace(f.ID) == "" {
				return fmt.Errorf("finding[%d]: id is required", i)
			}
		}
	case KindExtract:
		if err := m.rejectScalars("fix", boolStr(m.Fix != nil), "lang", m.Lang, "pattern", m.Pattern, "emit", m.Emit, "source", m.Source, "destination", m.Destination, "error", m.Error); err != nil {
			return err
		}
		if m.ExpectMatchCount != nil {
			return fmt.Errorf("extract: unexpected expect_match_count")
		}
		if err := m.rejectTables(true, true, true, false); err != nil {
			return err
		}
		if len(m.Atom) == 0 {
			return fmt.Errorf("extract: need [[atom]]")
		}
		for i, a := range m.Atom {
			if strings.TrimSpace(a.Name) == "" || a.Reference != "" || a.StartByte != 0 || a.EndByte != 0 {
				return fmt.Errorf("extract: atom[%d] wants name only", i)
			}
		}
	case KindIngest:
		if err := m.rejectScalars("fix", boolStr(m.Fix != nil), "lang", m.Lang, "pattern", m.Pattern, "emit", m.Emit, "source", m.Source, "destination", m.Destination, "error", m.Error); err != nil {
			return err
		}
		if m.ExpectMatchCount != nil {
			return fmt.Errorf("ingest: unexpected expect_match_count")
		}
		if len(m.Finding) > 0 {
			return fmt.Errorf("ingest: unexpected [[finding]]")
		}
		if len(m.Atom) == 0 && len(m.File) == 0 {
			return fmt.Errorf("ingest: need [[file]] or [[atom]]")
		}
		for i, a := range m.Atom {
			if a.Name != "" || strings.TrimSpace(a.Reference) == "" {
				return fmt.Errorf("ingest: atom[%d] wants reference, not name", i)
			}
		}
	case KindGrep:
		if err := m.rejectScalars("fix", boolStr(m.Fix != nil), "emit", m.Emit, "source", m.Source, "destination", m.Destination, "error", m.Error); err != nil {
			return err
		}
		if strings.TrimSpace(m.Pattern) == "" {
			return fmt.Errorf("grep: pattern is required")
		}
		if err := m.rejectTables(true, true, true, true); err != nil {
			return err
		}
	case KindRewrite:
		if err := m.rejectScalars("fix", boolStr(m.Fix != nil), "source", m.Source, "destination", m.Destination, "error", m.Error); err != nil {
			return err
		}
		if m.ExpectMatchCount != nil {
			return fmt.Errorf("rewrite: unexpected expect_match_count")
		}
		if strings.TrimSpace(m.Pattern) == "" || strings.TrimSpace(m.Emit) == "" {
			return fmt.Errorf("rewrite: pattern and emit are required")
		}
		if err := m.rejectTables(true, true, true, true); err != nil {
			return err
		}
	case KindMv:
		if err := m.rejectScalars("fix", boolStr(m.Fix != nil), "lang", m.Lang, "pattern", m.Pattern, "emit", m.Emit); err != nil {
			return err
		}
		if m.ExpectMatchCount != nil {
			return fmt.Errorf("mv: unexpected expect_match_count")
		}
		// source/destination may be empty (idempotent/empty-path error cases).
		if err := m.rejectTables(true, true, true, true); err != nil {
			return err
		}
	default:
		if m.Kind == "" {
			return fmt.Errorf("kind is required")
		}
		return fmt.Errorf("unknown kind %q", m.Kind)
	}
	return nil
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return ""
}

func (m CaseMeta) rejectScalars(pairs ...string) error {
	if len(pairs)%2 != 0 {
		return fmt.Errorf("rejectScalars: odd pairs")
	}
	for i := 0; i < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i+1]) != "" {
			return fmt.Errorf("%s: unexpected %s", m.Kind, pairs[i])
		}
	}
	return nil
}

func (m CaseMeta) rejectTables(finding, file, use, atom bool) error {
	if finding && len(m.Finding) > 0 {
		return fmt.Errorf("%s: unexpected [[finding]]", m.Kind)
	}
	if file && len(m.File) > 0 {
		return fmt.Errorf("%s: unexpected [[file]]", m.Kind)
	}
	if use && (len(m.Use) > 0 || len(m.Alias) > 0) {
		return fmt.Errorf("%s: unexpected [[use]] or [[alias]]", m.Kind)
	}
	if atom && len(m.Atom) > 0 {
		return fmt.Errorf("%s: unexpected [[atom]]", m.Kind)
	}
	return nil
}

// wantsFix reports whether the case should apply fixes and compare expected/.
func (c Case) wantsFix() bool {
	if c.Meta.Fix != nil {
		return *c.Meta.Fix
	}
	return c.Expected != ""
}

// CaseResult is the outcome of RunCase.
type CaseResult struct {
	Name     string
	Findings []report.Finding
	// Failures are human messages for assertion failures (empty = pass).
	Failures []string
}

// RunCaseOptions control a fixture run.
type RunCaseOptions struct {
	// PackRoot is the directory used for ListPackScripts (repo root).
	// Empty → parent of .patlint that contains this case's testdata.
	PackRoot string
	// WorkDir is used as a temp copy of scenario for fix runs.
	// Empty → create a temporary directory (removed by caller if set via Temp).
	WorkDir string
}

// RunCase executes one fixture according to c.Meta.Kind.
func RunCase(ctx context.Context, c Case, opts RunCaseOptions) (CaseResult, error) {
	res := CaseResult{Name: c.Name}
	switch c.Meta.Kind {
	case KindRun:
		return runKindRun(ctx, c, opts)
	case KindExtract:
		return runKindExtract(ctx, c)
	case KindIngest:
		return runKindIngest(ctx, c)
	case KindGrep:
		return runKindGrep(ctx, c)
	case KindRewrite:
		return runKindRewrite(ctx, c, opts)
	case KindMv:
		return runKindMv(ctx, c, opts)
	default:
		return res, fmt.Errorf("unknown kind %q", c.Meta.Kind)
	}
}

func preludeWalker(root string) (*walker.Walker, error) {
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		return nil, err
	}
	return walker.NewWalker(project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
}

func runKindRun(ctx context.Context, c Case, opts RunCaseOptions) (CaseResult, error) {
	res := CaseResult{Name: c.Name}
	packRoot := opts.PackRoot
	if packRoot == "" {
		packRoot = filepath.Clean(lewpath.New(c.Dir, "..", "..", "..").String())
	}
	packRoot, err := filepath.Abs(packRoot)
	if err != nil {
		return res, err
	}

	scripts, err := ListPackScripts(packRoot)
	if err != nil {
		return res, err
	}
	var progs []*Program
	for _, sp := range scripts {
		p, err := LoadFile(sp)
		if err != nil {
			return res, err
		}
		progs = append(progs, p)
	}
	prog := MergePrograms(strings.Join(scripts, "+"), progs)
	if prog.Path == "" {
		prog.Path = "<builtin>"
	}
	prog = EnsureDeadImports(prog)

	runRes, err := Run(ctx, project.NewSession(c.Scenario).WithEngine(ccgo.Engine{}), prog, Options{Paths: []string{"."}})
	if err != nil {
		return res, err
	}
	res.Findings = runRes.Findings
	res.Failures = append(res.Failures, AssertFindings(c.Meta.Finding, runRes.Findings)...)

	if !c.wantsFix() {
		return res, nil
	}
	if c.Expected == "" {
		res.Failures = append(res.Failures, "fix requested but expected/ is missing")
		return res, nil
	}

	work, cleanup, err := workCopy(ctx, c, opts)
	if err != nil {
		return res, err
	}
	if cleanup {
		defer os.RemoveAll(work)
	}
	fixRes, err := Run(ctx, project.NewSession(work).WithEngine(ccgo.Engine{}), prog, Options{Paths: []string{"."}})
	if err != nil {
		return res, err
	}
	if len(fixRes.ApplyEdits) > 0 {
		if err := project.ApplyEdits(ctx, work, fixRes.ApplyEdits); err != nil {
			return res, fmt.Errorf("apply fixes: %w", err)
		}
	}
	res.Failures = append(res.Failures, DiffTrees(ctx, c.Expected, work)...)
	return res, nil
}

func runKindExtract(ctx context.Context, c Case) (CaseResult, error) {
	res := CaseResult{Name: c.Name}
	w, err := preludeWalker(c.Scenario)
	if err != nil {
		return res, err
	}
	got := map[string]bool{}
	err = w.WalkExtracts(ctx, ingest.SourceProject(c.Scenario), func(fe *project.FileExtract) bool {
		if fe == nil {
			return true
		}
		for _, a := range fe.Atoms {
			got[a.Name] = true
		}
		return true
	})
	if err != nil {
		return res, err
	}
	for _, a := range c.Meta.Atom {
		if !got[a.Name] {
			res.Failures = append(res.Failures, fmt.Sprintf("missing atom %q", a.Name))
		}
	}
	return res, nil
}

func runKindIngest(ctx context.Context, c Case) (CaseResult, error) {
	res := CaseResult{Name: c.Name}
	w, err := preludeWalker(c.Scenario)
	if err != nil {
		return res, err
	}
	got, err := w.Load(ctx, ingest.SourceProject(c.Scenario), ingest.MaterializeOptions{ExpandImports: true})
	if err != nil {
		return res, err
	}
	want := c.Meta.wantResult()
	ingest.SortResult(&want)
	ingest.SortResult(got)
	wantJSON, err := json.MarshalIndent(&want, "", "  ")
	if err != nil {
		return res, err
	}
	gotJSON, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		return res, err
	}
	if string(wantJSON) != string(gotJSON) {
		res.Failures = append(res.Failures, "ingest graph mismatch")
	}
	return res, nil
}

func (m CaseMeta) wantResult() project.Result {
	r := project.Result{
		Files:   make([]project.File, len(m.File)),
		Atoms:   make([]project.Atom, len(m.Atom)),
		Uses:    make([]project.Use, len(m.Use)),
		Aliases: make([]project.Alias, len(m.Alias)),
	}
	for i, f := range m.File {
		r.Files[i] = project.File{Language: f.Language, Path: f.Path}
	}
	for i, a := range m.Atom {
		r.Atoms[i] = project.Atom{Reference: a.Reference, StartByte: a.StartByte, EndByte: a.EndByte}
	}
	for i, u := range m.Use {
		r.Uses[i] = project.Use{
			Reference: u.Reference, StartByte: u.StartByte, EndByte: u.EndByte,
			Target: u.Target, ViaImportAlias: u.ViaImportAlias,
		}
	}
	for i, a := range m.Alias {
		r.Aliases[i] = project.Alias{
			Reference: a.Reference, StartByte: a.StartByte, EndByte: a.EndByte, Target: a.Target,
		}
	}
	return r
}

func runKindGrep(ctx context.Context, c Case) (CaseResult, error) {
	res := CaseResult{Name: c.Name}
	op, err := PatternOp(c)
	if err != nil {
		return res, err
	}
	w, err := preludeWalker(c.Scenario)
	if err != nil {
		return res, err
	}
	out, err := w.Run(ctx, op, pattern.RunOptions{})
	if err != nil {
		return res, err
	}
	if c.Meta.ExpectMatchCount != nil && len(out.Matches) != *c.Meta.ExpectMatchCount {
		res.Failures = append(res.Failures, fmt.Sprintf("match count: got %d want %d", len(out.Matches), *c.Meta.ExpectMatchCount))
	}
	return res, nil
}

func runKindRewrite(ctx context.Context, c Case, opts RunCaseOptions) (CaseResult, error) {
	res := CaseResult{Name: c.Name}
	if c.Expected == "" {
		res.Failures = append(res.Failures, "rewrite needs expected/")
		return res, nil
	}
	op, err := PatternOp(c)
	if err != nil {
		return res, err
	}
	work, cleanup, err := workCopy(ctx, c, opts)
	if err != nil {
		return res, err
	}
	if cleanup {
		defer os.RemoveAll(work)
	}
	w, err := preludeWalker(work)
	if err != nil {
		return res, err
	}
	if _, err := w.Apply(ctx, op, pattern.RunOptions{}); err != nil {
		return res, err
	}
	res.Failures = append(res.Failures, DiffTrees(ctx, c.Expected, work)...)
	return res, nil
}

func runKindMv(ctx context.Context, c Case, opts RunCaseOptions) (CaseResult, error) {
	res := CaseResult{Name: c.Name}
	work, cleanup, err := workCopy(ctx, c, opts)
	if err != nil {
		return res, err
	}
	if cleanup {
		defer os.RemoveAll(work)
	}
	w, err := preludeWalker(work)
	if err != nil {
		return res, err
	}
	plan, err := w.Rename(ctx, work, c.Meta.Source, c.Meta.Destination)
	if c.Meta.Error != "" {
		if err == nil {
			res.Failures = append(res.Failures, fmt.Sprintf("rename: want error containing %q, got nil", c.Meta.Error))
			return res, nil
		}
		if !strings.Contains(err.Error(), c.Meta.Error) {
			res.Failures = append(res.Failures, fmt.Sprintf("rename: want error containing %q, got %v", c.Meta.Error, err))
		}
		return res, nil
	}
	if err != nil {
		res.Failures = append(res.Failures, err.Error())
		return res, nil
	}
	if err := ingest.ApplyPlan(ctx, work, plan); err != nil {
		res.Failures = append(res.Failures, fmt.Sprintf("apply plan: %v", err))
		return res, nil
	}
	if c.Expected == "" {
		res.Failures = append(res.Failures, "mv needs expected/")
		return res, nil
	}
	res.Failures = append(res.Failures, DiffTrees(ctx, c.Expected, work)...)
	return res, nil
}

func workCopy(ctx context.Context, c Case, opts RunCaseOptions) (work string, cleanup bool, err error) {
	work = opts.WorkDir
	if work == "" {
		work, err = os.MkdirTemp("", "rft-test-"+c.Name+"-*")
		if err != nil {
			return "", false, err
		}
		cleanup = true
	}
	if err := copyTree(ctx, c.Scenario, work); err != nil {
		if cleanup {
			os.RemoveAll(work)
		}
		return "", false, fmt.Errorf("copy scenario: %w", err)
	}
	return work, cleanup, nil
}

// PatternOp builds a grep/rewrite pattern.Op from a case.
func PatternOp(c Case) (pattern.Op, error) {
	op := pattern.Op{
		Mode:             c.Meta.Kind,
		Lang:             c.Meta.Lang,
		Description:      c.Meta.Description,
		Pattern:          c.Meta.Pattern,
		ExpectMatchCount: c.Meta.ExpectMatchCount,
	}
	if c.Meta.Kind == KindRewrite {
		em := c.Meta.Emit
		op.Replacement = &em
	}
	if err := op.ResolvePatternIR(); err != nil {
		return op, err
	}
	if op.Mode == KindRewrite {
		if err := op.PrepareRewrite(); err != nil {
			return op, err
		}
	}
	return op, nil
}

// AssertFindings checks TOML ground truth against actual findings.
// Findings are grouped by rule id. For each id present in want, every want
// row must match a distinct actual finding (partial field equality). Extra
// actual findings (any id) are ignored.
func AssertFindings(want []WantFinding, actual []report.Finding) []string {
	if len(want) == 0 {
		return nil
	}
	wantBy := map[string][]WantFinding{}
	var order []string
	for _, w := range want {
		id := w.ID
		if _, ok := wantBy[id]; !ok {
			order = append(order, id)
		}
		wantBy[id] = append(wantBy[id], w)
	}
	actBy := map[string][]report.Finding{}
	for _, a := range actual {
		actBy[a.RuleID] = append(actBy[a.RuleID], a)
	}

	var fails []string
	for _, id := range order {
		ws := wantBy[id]
		as := actBy[id]
		used := make([]bool, len(as))
		for i, w := range ws {
			matched := false
			for j, a := range as {
				if used[j] {
					continue
				}
				if findingMatches(w, a) {
					used[j] = true
					matched = true
					break
				}
			}
			if !matched {
				fails = append(fails, fmt.Sprintf(
					"rule %q: findings[%d] not matched (want %s; have %d actual with this id)",
					id, i, formatWant(w), len(as),
				))
			}
		}
	}
	return fails
}

func findingMatches(w WantFinding, a report.Finding) bool {
	if w.ID != "" && w.ID != a.RuleID {
		return false
	}
	path := w.Path
	if path == "" {
		path = w.File
	}
	if path != "" {
		got := filepath.ToSlash(a.File)
		want := filepath.ToSlash(path)
		if got != want && !strings.HasSuffix(got, "/"+want) {
			return false
		}
	}
	if w.Line != nil && *w.Line != a.Line {
		return false
	}
	if w.Column != nil && *w.Column != a.Column {
		return false
	}
	if w.EndLine != nil && *w.EndLine != a.EndLine {
		return false
	}
	if w.EndCol != nil && *w.EndCol != a.EndCol {
		return false
	}
	if w.Message != "" && w.Message != a.Message {
		return false
	}
	if w.Level != "" {
		lvl, err := report.ParseLevel(w.Level)
		if err != nil || lvl != a.Level {
			return false
		}
	}
	if w.Snippet != "" && w.Snippet != a.Snippet {
		return false
	}
	return true
}

func formatWant(w WantFinding) string {
	var b strings.Builder
	b.WriteString("id=" + w.ID)
	path := w.Path
	if path == "" {
		path = w.File
	}
	if path != "" {
		b.WriteString(" path=" + path)
	}
	if w.Line != nil {
		fmt.Fprintf(&b, " line=%d", *w.Line)
	}
	if w.Column != nil {
		fmt.Fprintf(&b, " column=%d", *w.Column)
	}
	if w.Snippet != "" {
		b.WriteString(" snippet=" + w.Snippet)
	}
	return b.String()
}

// DiffTrees compares every file under expected/ to the same relative path under gotRoot.
// Extra files under gotRoot are ignored. Missing or different content → failure messages.
func DiffTrees(ctx context.Context, expectedRoot, gotRoot string) []string {
	var fails []string
	err := filepath.WalkDir(expectedRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(expectedRoot, path)
		if err != nil {
			return err
		}
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		gotPath := lewpath.New(gotRoot, rel).String()
		got, err := os.ReadFile(gotPath)
		if err != nil {
			fails = append(fails, fmt.Sprintf("expected %s: missing in result: %v", filepath.ToSlash(rel), err))
			return nil
		}
		if !bytes.Equal(want, got) {
			fails = append(fails, contentMismatch(filepath.ToSlash(rel), want, got))
		}
		return nil
	})
	if err != nil {
		fails = append(fails, err.Error())
	}
	return fails
}

type assertT struct{ msg string }

func (t *assertT) Helper() {}

func (t *assertT) Errorf(format string, args ...any) {
	t.msg = stripTestifyTrace(fmt.Sprintf(format, args...))
}

func stripTestifyTrace(msg string) string {
	var keep []string
	for _, line := range strings.Split(msg, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "Error Trace:") {
			continue
		}
		if (strings.Contains(line, ".go:") || strings.Contains(line, ".s:")) && !strings.Contains(trim, "Error:") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

func contentMismatch(rel string, want, got []byte) string {
	if bytes.IndexByte(want, 0) >= 0 || bytes.IndexByte(got, 0) >= 0 {
		return fmt.Sprintf("expected %s: binary mismatch (%d vs %d bytes)", rel, len(want), len(got))
	}
	var t assertT
	assert.Equal(&t, string(want), string(got), rel)
	if t.msg == "" {
		return fmt.Sprintf("expected %s: content mismatch", rel)
	}
	return t.msg
}

func copyTree(ctx context.Context, src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := lewpath.New(dst, rel).String()
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
