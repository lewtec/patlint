package walker

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	refpkg "github.com/lewtec/patlint/pkg/reference"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/store"
)

// Walker is the stateful read/nav spine: always NewWalker(sess, vm).
type Walker struct {
	Sess *project.Session
	VM   *pattern.LispVM
}

// NewWalker pairs a view with frozen policy. Both required.
func NewWalker(sess *project.Session, vm *pattern.LispVM) (*Walker, error) {
	if sess == nil {
		return nil, ingest.ErrNilSession
	}
	if vm == nil {
		return nil, fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	if sess.Engine() == nil {
		return nil, sitter.ErrNilEngine
	}
	if vm.Packs() != nil && vm.Packs().Program() != nil {
		if err := vm.Packs().Program().ValidateGrammars(sess.Engine()); err != nil {
			return nil, err
		}
	}
	return &Walker{Sess: sess, VM: vm}, nil
}

// HostLanguage is VM PackQueries AttributeHost for relPath (slash-relative).
func (w *Walker) HostLanguage(relPath string) (string, bool) {
	if w == nil || w.VM == nil {
		return "", false
	}
	return w.VM.HostLanguage(relPath)
}

// LanguageForReference is VM host language for a product ref.
// Path claims win; else a provider that packs join as a language; else the
// provider's HighlightLanguage. A directory path peeks at child sources.
func (w *Walker) LanguageForReference(ref ingest.Reference) string {
	if w == nil {
		return ""
	}
	if ref.Path != "" {
		rel := strings.TrimPrefix(filepath.ToSlash(ref.Path), "./")
		if lang, ok := w.HostLanguage(rel); ok {
			return lang
		}
		if base := filepath.Base(rel); base != rel {
			if lang, ok := w.HostLanguage(base); ok {
				return lang
			}
		}
		if lang := w.hostLanguageInDir(rel); lang != "" {
			return lang
		}
	}
	p := ref.Provider
	if p == "" || p == "path" {
		return ""
	}
	if w.VM != nil && w.VM.Packs() != nil && w.VM.Packs().FamilyForLanguage(p) != "" {
		return p
	}
	if prov, ok := refpkg.ProviderForName(p); ok {
		if h, ok := prov.(ingest.HighlightLanguageProvider); ok {
			if lang := h.HighlightLanguage(); lang != "" {
				return lang
			}
		}
	}
	return ""
}

func (w *Walker) hostLanguageInDir(rel string) string {
	if w.Sess == nil || w.Sess.FS == nil || rel == "" || rel == "." {
		return ""
	}
	entries, err := fs.ReadDir(w.Sess.FS, rel)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if lang, ok := w.HostLanguage(path.Join(rel, e.Name())); ok {
			return lang
		}
		if lang, ok := w.HostLanguage(e.Name()); ok {
			return lang
		}
	}
	return ""
}

// WalkExtracts streams extracts using this Walker (VM host + extract).
func (w *Walker) WalkExtracts(ctx context.Context, src ingest.ExtractSource, yield func(*project.FileExtract) bool) error {
	if w == nil || w.Sess == nil {
		return ingest.ErrNilSession
	}
	if w.VM == nil {
		return fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	if yield == nil {
		return ingest.ErrWalkNilYield
	}
	src.Session = w.Sess
	src.Policy = w.VM
	st := store.New()
	return ingest.WalkStore(ctx, src, st, func(path string) bool {
		fe := store.ProjectExtract(st, path)
		if fe == nil {
			return true
		}
		return yield(fe)
	})
}

// LoadStore is the closed graph in the store (strata 1–2). Load projects this.
func (w *Walker) LoadStore(ctx context.Context, src ingest.ExtractSource, opts ingest.MaterializeOptions) (*store.Store, error) {
	if w == nil || w.Sess == nil {
		return nil, ingest.ErrNilSession
	}
	if w.VM == nil {
		return nil, fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	if opts.FS == nil {
		opts.FS = src.FS
	}
	src.Session = w.Sess
	src.Policy = w.VM
	opts.Session = w.Sess
	opts.Policy = w.VM
	return ingest.MaterializeStore(ctx, src, opts)
}

// Load is the Result view of LoadStore.
func (w *Walker) Load(ctx context.Context, src ingest.ExtractSource, opts ingest.MaterializeOptions) (*project.Result, error) {
	st, err := w.LoadStore(ctx, src, opts)
	if err != nil {
		return nil, err
	}
	return store.Project(st, w.VM.Families()), nil
}

// LoadProviderScope loads a provider ScopeTarget: file hop, or non-recursive dir.
func (w *Walker) LoadProviderScope(ctx context.Context, scope ingest.ProviderScopeTarget) (*project.Result, error) {
	if w == nil || w.Sess == nil {
		return nil, ingest.ErrNilSession
	}
	if scope.Dir == "" {
		return nil, ingest.ErrProviderScopeEmptyDir
	}
	if scope.File != "" {
		abs := scope.File
		if !filepath.IsAbs(abs) {
			abs = lewpath.New(scope.Dir, scope.File).String()
		}
		return w.Load(ctx, ingest.SourceHop(scope.Dir, abs), ingest.MaterializeOptions{})
	}
	return w.Load(ctx, ingest.SourceDir(scope.Dir, "", false), ingest.MaterializeOptions{})
}

// WalkAtoms lists atoms via WalkExtracts with VM PackQueries.
func (w *Walker) WalkAtoms(ctx context.Context, dir, reference string, opts ingest.ListOptions, yield func(ingest.AtomInfo) bool) error {
	if w == nil || w.Sess == nil {
		return ingest.ErrNilSession
	}
	if w.VM == nil {
		return fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	opts.Session = w.Sess
	opts.Policy = w.VM
	return ingest.WalkAtoms(ctx, dir, reference, opts, yield)
}

// PackageSourceFiles lists hop targets for a package visit (VM DirectoryModule).
func (w *Walker) PackageSourceFiles(ctx context.Context, abs string, isDir bool) []string {
	if w == nil || w.Sess == nil || w.VM == nil {
		return nil
	}
	dirModule := false
	if !isDir {
		rel := filepath.ToSlash(abs)
		if r, err := filepath.Rel(w.Sess.Root, abs); err == nil {
			rel = filepath.ToSlash(r)
		}
		if lang, ok := w.HostLanguage(rel); ok {
			dirModule = w.VM.RulesForLanguage(lang).DirectoryModule
		}
	}
	return ingest.PackageSourceFiles(ctx, w.Sess, abs, isDir, dirModule, w.VM)
}

// Grep runs a sexp matcher on this Walker (VM PackQueries).
func (w *Walker) Grep(ctx context.Context, lang, pat string, paths []string) ([]pattern.Match, error) {
	if w == nil || w.Sess == nil {
		return nil, ingest.ErrNilSession
	}
	op, err := pattern.OpFromCLI("grep", lang, pat, "")
	if err != nil {
		return nil, err
	}
	var out []pattern.Match
	err = w.Stream(ctx, op, pattern.StreamOptions{
		Paths: paths,
		OnMatch: func(m pattern.Match, _ []byte) bool {
			out = append(out, m)
			return true
		},
	})
	return out, err
}
