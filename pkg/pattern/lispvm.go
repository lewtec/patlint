package pattern

import (
	"context"
	"fmt"
	"github.com/lewtec/patlint/pkg/projectfs"
	"io/fs"
	"path"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
)

// LispVM is frozen compile of combined lisp (SPEC.md Session, FS, and stacked views).
// New(sources…) expands once. Queries decode. Run/mv later.
type LispVM struct {
	p     *Packs
	extra []ExtraForm
}

// ExtraForm is one expanded top-level form from New srcs (not prelude).
// Head is the original symbol; Data is the expanded form (nil for def).
type ExtraForm struct {
	File  string
	Index int
	Head  string
	Data  any
}

// Source is lisp input. File / dir / string parse to forms; FromForms is the AST.
type Source interface {
	name() string
	files() ([]PackFile, error)
}

type fileSource struct{ path string }

func (s fileSource) name() string { return s.path }
func (s fileSource) files() ([]PackFile, error) {
	b, err := (projectfs.OS{}).ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	return []PackFile{{Name: s.path, Src: string(b)}}, nil
}

// FromFile loads one .rft path.
func FromFile(path string) Source { return fileSource{path: path} }

type dirSource struct {
	dir  string
	glob string
}

func (s dirSource) name() string { return s.dir }
func (s dirSource) files() ([]PackFile, error) {
	glob := s.glob
	if glob == "" {
		glob = "*.rft"
	}
	ents, err := (projectfs.OS{}).ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []PackFile
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		ok, err := path.Match(glob, e.Name())
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		fp := lewpath.New(s.dir, e.Name()).String()
		b, err := (projectfs.OS{}).ReadFile(fp)
		if err != nil {
			return nil, err
		}
		out = append(out, PackFile{Name: fp, Src: string(b)})
	}
	return out, nil
}

// FromDir loads names matching glob (default *.rft) in dir.
func FromDir(dir, glob string) Source { return dirSource{dir: dir, glob: glob} }

type stringSource struct{ id, src string }

func (s stringSource) name() string { return s.id }
func (s stringSource) files() ([]PackFile, error) {
	return []PackFile{{Name: s.id, Src: s.src}}, nil
}

// FromString is named sexp text.
func FromString(name, src string) Source { return stringSource{id: name, src: src} }

type formsSource struct {
	id    string
	forms []any
}

func (s formsSource) name() string { return s.id }
func (s formsSource) files() ([]PackFile, error) {
	var b strings.Builder
	for i, f := range s.forms {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(formatData(f))
	}
	return []PackFile{{Name: s.id, Src: b.String()}}, nil
}

// FromForms passes sexp AST (lists/atoms). Still goes through one expand at New.
func FromForms(name string, forms ...any) Source { return formsSource{id: name, forms: forms} }

type fsSource struct {
	fsys fs.FS
	dir  string
}

func (s fsSource) name() string {
	if s.dir == "" {
		return "."
	}
	return s.dir
}

func (s fsSource) files() ([]PackFile, error) {
	return packFilesFS(s.fsys, s.dir)
}

// FromFS loads every *.rft under dir in fsys (sorted).
func FromFS(fsys fs.FS, dir string) Source { return fsSource{fsys: fsys, dir: dir} }

func formatData(v any) string {
	switch x := v.(type) {
	case string:
		if strings.ContainsAny(x, " \t\n()\"") {
			return fmt.Sprintf("%q", x)
		}
		return x
	case []any:
		var b strings.Builder
		b.WriteByte('(')
		for i, el := range x {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(formatData(el))
		}
		b.WriteByte(')')
		return b.String()
	default:
		return fmt.Sprint(v)
	}
}

// New expands packs from fsys (every *.rft at the root) plus extra lisp, then freezes.
// Product callers pass prelude.FS. fsys may be nil when only extra sources are given.
// No files → error.
func New(fsys fs.FS, more ...Source) (*LispVM, error) {
	var files []PackFile
	var user []PackFile
	if fsys != nil {
		got, err := packFilesFS(fsys, ".")
		if err != nil {
			return nil, fmt.Errorf("lispvm: %w", err)
		}
		files = append(files, got...)
	}
	for _, s := range more {
		if s == nil {
			continue
		}
		got, err := s.files()
		if err != nil {
			return nil, fmt.Errorf("lispvm %s: %w", s.name(), err)
		}
		files = append(files, got...)
		user = append(user, got...)
	}
	if len(user) == 0 {
		user = files
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: lispvm: no sources", ErrExtract)
	}
	p, err := LoadFiles(files)
	if err != nil {
		return nil, err
	}
	extra, err := expandExtra(user, p.env)
	if err != nil {
		return nil, err
	}
	return &LispVM{p: p, extra: extra}, nil
}

func expandExtra(files []PackFile, env expandEnv) ([]ExtraForm, error) {
	var out []ExtraForm
	for _, f := range files {
		forms, err := ParseSexpDataFile(f.Src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		for i, form := range forms {
			list, ok := form.([]any)
			if !ok || len(list) == 0 {
				return nil, fmt.Errorf("%s: form %d: want list", f.Name, i)
			}
			head, ok := list[0].(string)
			if !ok {
				return nil, fmt.Errorf("%s: form %d: list head must be symbol", f.Name, i)
			}
			rec := ExtraForm{File: f.Name, Index: i, Head: head}
			if head == "def" {
				out = append(out, rec)
				continue
			}
			expanded, err := Expand(form, env)
			if err != nil {
				return nil, fmt.Errorf("%s: form %d: %w", f.Name, i, err)
			}
			rec.Data = expanded
			out = append(out, rec)
		}
	}
	return out, nil
}

// ExtraForms is expanded top-level forms from New srcs (prelude omitted).
func (vm *LispVM) ExtraForms() []ExtraForm {
	if vm == nil || len(vm.extra) == 0 {
		return nil
	}
	out := make([]ExtraForm, len(vm.extra))
	copy(out, vm.extra)
	return out
}

// Packs is the compiled extract program (query).
func (vm *LispVM) Packs() *Packs {
	if vm == nil {
		return nil
	}
	return vm.p
}

// HostLanguage is the extract-view host language for rel.
func (vm *LispVM) HostLanguage(rel string) (string, bool) {
	if vm == nil || vm.p == nil {
		return "", false
	}
	lang, ok, err := vm.p.HostLanguage(rel)
	if err != nil || !ok {
		return "", false
	}
	return lang, true
}

// Run applies rewrite heads on sess (Write-only PatchFS). No rematerialize. No disk write.
// paths empty means the whole Root. WriteBack is a separate commit.
func (vm *LispVM) Run(ctx context.Context, sess *project.Session, paths ...string) (*project.Session, error) {
	if vm == nil {
		return nil, fmt.Errorf("%w: lispvm: nil", ErrExtract)
	}
	if sess == nil {
		return nil, ingest.ErrNilSession
	}
	ops, err := vm.rewriteOps()
	if err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return sess, nil
	}
	cur := sess
	for _, op := range ops {
		writes := map[string][]byte{}
		err = Stream(ctx, cur, vm, op, StreamOptions{
			Paths: paths,
			OnFile: func(rel string, _ []Match, fileEdits []project.Edit, source []byte) bool {
				if len(fileEdits) == 0 {
					return true
				}
				next := project.ApplyEditsInMemory(source, fileEdits)
				if string(next) != string(source) {
					writes[rel] = next
				}
				return true
			},
		})
		if err != nil {
			return nil, err
		}
		if len(writes) == 0 {
			continue
		}
		parent := cur.FS
		if parent == nil {
			opened, err := lewpath.Open(cur.Root)
			if err != nil {
				return nil, err
			}
			parent = opened
		}
		cur = cur.WithFS(project.NewPatchFS(parent, writes))
	}
	return cur, nil
}
