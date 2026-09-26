package walker

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/projectfs"
	"github.com/lewtec/patlint/pkg/store"
)

// NavigateAround is hop+seed around one file (goto/hover). Both legs expand
// imports so one-hop targets resolve under overlays.
func (w *Walker) NavigateAround(ctx context.Context, root string, fsys projectfs.FS, seedAbs string) (*project.Result, error) {
	st, err := w.aroundStore(ctx, root, fsys, seedAbs)
	if err != nil {
		return nil, err
	}
	return store.Project(st, w.VM.Families()), nil
}

func (w *Walker) aroundStore(ctx context.Context, root string, fsys projectfs.FS, seedAbs string) (*store.Store, error) {
	if w == nil || w.Sess == nil {
		return nil, ingest.ErrNilSession
	}
	if w.VM == nil {
		return nil, fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	if fsys == nil {
		fsys = projectfs.OS{}
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}
	if !filepath.IsAbs(seedAbs) {
		seedAbs = lewpath.New(rootAbs, filepath.FromSlash(strings.TrimPrefix(seedAbs, "./"))).String()
	}
	st := store.New()
	opts := ingest.MaterializeOptions{ExpandImports: true, FS: fsys, Session: w.Sess, Policy: w.VM}
	if err := w.ingestInto(ctx, st, ingest.SourceHop(rootAbs, seedAbs).WithFS(fsys), opts); err != nil {
		return nil, err
	}
	if err := w.ingestInto(ctx, st, ingest.SourceSeed(rootAbs, seedAbs).WithFS(fsys), opts); err != nil {
		return nil, err
	}
	if err := w.closeStore(ctx, st, rootAbs, fsys); err != nil {
		return nil, err
	}
	return st, nil
}

func (w *Walker) ingestInto(ctx context.Context, st *store.Store, src ingest.ExtractSource, opts ingest.MaterializeOptions) error {
	src.Session = w.Sess
	src.Policy = w.VM
	if src.FS == nil {
		src.FS = opts.FS
	}
	return ingest.WalkStore(ctx, src, st, nil)
}

func (w *Walker) closeStore(ctx context.Context, st *store.Store, rootAbs string, fsys projectfs.FS) error {
	if st == nil {
		return nil
	}
	if err := ingest.ExpandImportsInto(ctx, st, w.Sess, rootAbs, fsys, w.VM); err != nil {
		return err
	}
	st.Reset(store.RelationBinds)
	_, err := ingest.EvalStore(ctx, rootAbs, st, w.VM)
	return err
}

// NavigateReference resolves ref for navigation. path → aroundStore; other
// providers → ResolveScopeTarget + LoadStore. Result is a projection of the store.
func (w *Walker) NavigateReference(ctx context.Context, root string, fsys projectfs.FS, base *project.Result, ref ingest.Reference) (*project.Result, ingest.Reference) {
	st := store.New()
	if base != nil {
		store.LoadProjected(st, base)
	}
	st, ref = w.NavigateReferenceStore(ctx, root, fsys, st, ref)
	return store.Project(st, w.VM.Families()), ref
}

// NavigateReferenceStore is NavigateReference on a live store (hops ingest into st).
func (w *Walker) NavigateReferenceStore(ctx context.Context, root string, fsys projectfs.FS, st *store.Store, ref ingest.Reference) (*store.Store, ingest.Reference) {
	if st == nil {
		st = store.New()
	}
	if w == nil || w.Sess == nil {
		return st, ref
	}
	if fsys == nil {
		fsys = projectfs.OS{}
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}
	ref = ingest.NormalizePathReference(ref)
	if ref.Provider == "" {
		ref.Provider = "path"
	}

	const maxHops = 12
	seen := map[string]bool{}
	for hop := 0; hop < maxHops; hop++ {
		if err := ctx.Err(); err != nil {
			return st, ref
		}
		key := ref.String()
		if seen[key] {
			break
		}
		seen[key] = true

		view := store.Project(st, w.VM.Families())
		next := ingest.CanonicalizeInResult(view, ref)
		if ingest.EntityExactOK(view, next.String()) || (next.Name != "" && ingest.EntityAtPathSymbolOK(view, next)) {
			return st, next
		}

		if next.Provider == "path" || next.Provider == "" {
			abs := ingest.AbsPathForRef(rootAbs, next)
			if abs != "" {
				if !storeHasAbs(st, rootAbs, abs) {
					opts := ingest.MaterializeOptions{ExpandImports: true, FS: fsys, Session: w.Sess, Policy: w.VM}
					if err := w.ingestInto(ctx, st, ingest.SourceHop(rootAbs, abs).WithFS(fsys), opts); err != nil {
						return st, next
					}
					if err := w.ingestInto(ctx, st, ingest.SourceSeed(rootAbs, abs).WithFS(fsys), opts); err != nil {
						return st, next
					}
					if err := w.closeStore(ctx, st, rootAbs, fsys); err != nil {
						return st, next
					}
					view = store.Project(st, w.VM.Families())
				}
				next2 := ingest.CanonicalizeInResult(view, next)
				if next2.String() == next.String() && !ingest.EntityExactOK(view, next2.String()) {
					if aliasT, ok := ingest.FirstAliasTarget(view, next); ok {
						ref = aliasT
						continue
					}
					return st, next2
				}
				ref = next2
				continue
			}
		} else {
			nav, next2, ok := w.navigateProviderRefStore(ctx, rootAbs, next)
			if ok && nav != nil {
				st = mergeStores(st, nav)
				if err := w.closeStore(ctx, st, rootAbs, fsys); err != nil {
					return st, next2
				}
				view = store.Project(st, w.VM.Families())
				if ingest.EntityExactOK(view, next2.String()) || (next2.Name != "" && ingest.EntityAtPathSymbolOK(view, next2)) {
					return st, next2
				}
				if next.Name != "" {
					if ent, eok := ingest.SoleEntityNamed(view, next.Name); eok {
						return st, ingest.ParseReference(ent.Reference)
					}
				}
				if next2.String() == next.String() {
					return st, next2
				}
				ref = next2
				continue
			}
		}

		if next.String() == ref.String() {
			return st, next
		}
		ref = next
	}
	return st, ingest.CanonicalizeInResult(store.Project(st, w.VM.Families()), ref)
}

func (w *Walker) navigateProviderRefStore(ctx context.Context, rootAbs string, ref ingest.Reference) (*store.Store, ingest.Reference, bool) {
	if w == nil || ctx == nil {
		return nil, ref, false
	}
	scope, sok, err := ingest.NewResolver(rootAbs).ResolveScopeTarget(ctx, ref)
	if err != nil || !sok || scope.Dir == "" {
		return nil, ref, false
	}
	pkg, err := w.LoadStore(ctx, ingest.SourceDir(scope.Dir, "", false), ingest.MaterializeOptions{})
	if err != nil || pkg == nil {
		return nil, ref, false
	}
	view := ingest.AbsolutizeResultPaths(store.Project(pkg, w.VM.Families()), scope.Dir)
	out := store.New()
	store.LoadProjected(out, view)
	next := ingest.CanonicalizeInResult(view, ref)
	if next.Name == "" && ref.Name != "" {
		next.Name = ref.Name
	}
	if ref.Name != "" {
		if ent, eok := ingest.SoleEntityNamed(view, ref.Name); eok {
			return out, ingest.ParseReference(ent.Reference), true
		}
	}
	return out, next, true
}

func mergeStores(dst, src *store.Store) *store.Store {
	if dst == nil {
		return src
	}
	if src == nil {
		return dst
	}
	have := storeFiles(dst)
	for _, rel := range src.RelationNames() {
		for _, t := range src.Rows(rel) {
			if len(t) > 0 && have[normStoreFile(t[0])] && fileKeyedRel(rel) {
				continue
			}
			if rel == store.RelationUse || rel == store.RelationUseSegment || rel == store.RelationBinds {
				dst.Append(rel, t)
			} else {
				dst.Insert(rel, t)
			}
		}
	}
	return dst
}

func storeFiles(st *store.Store) map[string]bool {
	out := map[string]bool{}
	if st == nil {
		return out
	}
	for _, t := range st.Rows(store.RelationFile) {
		if len(t) > 0 {
			out[normStoreFile(t[0])] = true
		}
	}
	return out
}

func storeHasAbs(st *store.Store, rootAbs, abs string) bool {
	if st == nil || abs == "" {
		return false
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	want := normStoreFile(rel)
	for _, t := range st.Rows(store.RelationFile) {
		if len(t) > 0 && normStoreFile(t[0]) == want {
			return true
		}
	}
	return false
}

func fileKeyedRel(rel string) bool {
	switch rel {
	case store.RelationFile, store.RelationAtom, store.RelationScope, store.RelationUse, store.RelationUseSegment,
		store.RelationImport, store.RelationPackage, store.RelationReexport, store.RelationDefault,
		store.RelationDeclares, store.RelationAtomInFile, store.RelationBinds:
		return true
	}
	return false
}

func normStoreFile(p string) string {
	return strings.TrimPrefix(filepath.ToSlash(p), "./")
}

// DefinitionAt resolves the definition under cursor:
//
//	HitAtByteStore | IdentifierAt → NavigateReferenceStore → atom span
//
// Nil Project seeds the current file only; hops ingest into the same store.
func (w *Walker) DefinitionAt(ctx context.Context, q ingest.DefinitionQuery) (ingest.Definition, bool) {
	if w == nil || w.Sess == nil || ctx == nil {
		return ingest.Definition{}, false
	}
	if q.Root == "" {
		q.Root = "."
	}
	rootAbs, err := filepath.Abs(q.Root)
	if err != nil {
		rootAbs = q.Root
	}
	if q.FS == nil {
		q.FS = projectfs.OS{}
	}
	fileRel := strings.TrimPrefix(filepath.ToSlash(q.FileRel), "./")

	var st *store.Store
	if q.Project != nil {
		st = store.New()
		store.LoadProjected(st, q.Project)
	} else if fileRel != "" {
		seedAbs := lewpath.New(rootAbs, filepath.FromSlash(fileRel)).String()
		st, _ = w.aroundStore(ctx, rootAbs, q.FS, seedAbs)
	}
	if st == nil {
		st = store.New()
	}

	hit, ok := ingest.HitAtByteStore(st, fileRel, q.ByteOff)
	if !ok || hit.Reference == "" {
		tok, _, _ := ingest.IdentifierAt(q.Text, q.ByteOff)
		if tok == "" {
			return ingest.Definition{}, false
		}
		hit = ingest.Hit{Reference: ingest.AtomRef("./"+fileRel, tok)}
	}

	st, ref := w.NavigateReferenceStore(ctx, rootAbs, q.FS, st, ingest.ParseReference(hit.Reference))
	return ingest.DefinitionFromStore(rootAbs, st, ref)
}
