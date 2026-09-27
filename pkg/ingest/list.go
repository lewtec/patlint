package ingest

import (
	"context"
	"fmt"
	"github.com/lewtec/patlint/pkg/projectfs"
	"path"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/store"
)

// ListOptions controls symbol listing behavior.
type ListOptions struct {
	IncludeHidden bool
	Recursive     bool
	// Session is required (extract cache owner). Nil → ErrNilSession.
	Session *project.Session
	// Policy attributes/extracts. Nil attributes nothing.
	// Walker.WalkAtoms sets this from the LispVM.
	Policy PackQueries
}

// AtomInfo is one listed symbol with extracted metadata.
type AtomInfo struct {
	Atom      project.Atom
	Reference Reference
	Language  string
}

// WalkAtoms iterates symbols in a reference scope, invoking yield for each
// matching symbol. Returning false from yield stops early.
// Thin convenience over WalkExtracts (no Materialize, no ExpandImports).
// opts.Session required (nil → ErrNilSession).
func WalkAtoms(ctx context.Context, dir, reference string, opts ListOptions, yield func(AtomInfo) bool) error {
	if opts.Session == nil {
		return ErrNilSession
	}
	if yield == nil {
		return ErrWalkNilYield
	}
	ref := ParseReference(reference)
	src, ingestDir, err := listExtractSource(ctx, dir, ref, opts)
	if err != nil {
		return err
	}
	st := store.New()
	if err := WalkStore(ctx, src, st, nil); err != nil {
		return err
	}
	refPath, refIsDir := "", false
	if ref.Provider == "" || ref.Provider == "path" {
		refPath, refIsDir = listScopeForRef(ingestDir, ref)
	} else {
		refIsDir = true
	}
	return yieldAtomsFromStore(ctx, st, ref, refPath, refIsDir, providerListIngestRecursive(ref, opts), opts, yield)
}

// WalkListExtracts yields per-file extracts for the same scope WalkAtoms lists.
func WalkListExtracts(ctx context.Context, dir, reference string, opts ListOptions, yield func(*project.FileExtract) bool) error {
	if yield == nil {
		return ErrWalkNilYield
	}
	ref := ParseReference(reference)
	src, _, err := listExtractSource(ctx, dir, ref, opts)
	if err != nil {
		return err
	}
	return WalkExtracts(ctx, src, yield)
}

func listExtractSource(ctx context.Context, dir string, ref Reference, opts ListOptions) (ExtractSource, string, error) {
	if opts.Session == nil {
		return ExtractSource{}, "", ErrNilSession
	}
	ingestDir := dir
	refPath := ""
	refIsDir := false
	if ref.Provider != "" && ref.Provider != "path" {
		scope, ok, err := NewResolver("").ResolveScopeTarget(ctx, ref)
		if err != nil {
			return ExtractSource{}, "", err
		}
		if !ok {
			return ExtractSource{}, "", fmt.Errorf("%w %q", ErrListingNotSupported, ref.Provider)
		}
		ingestDir = scope.Dir
		refIsDir = true
	} else {
		refPath, refIsDir = listScopeForRef(ingestDir, ref)
	}
	if !refIsDir && refPath != "" {
		abs := ref.Path
		if !filepath.IsAbs(abs) {
			abs = lewpath.New(ingestDir, filepath.FromSlash(refPath)).String()
		}
		if a, err := filepath.Abs(abs); err == nil {
			abs = a
		}
		return ExtractSource{
			Kind:    ExtractHop,
			Root:    ingestDir,
			Paths:   []string{abs},
			Session: opts.Session,
			Policy:  opts.Policy,
		}, ingestDir, nil
	}
	return ExtractSource{
		Kind:      ExtractDir,
		Root:      ingestDir,
		Recursive: providerListIngestRecursive(ref, opts),
		Session:   opts.Session,
		Policy:    opts.Policy,
	}, ingestDir, nil
}

// WalkOutline WalkStores the listing scope and yields outline rows per file.
func WalkOutline(ctx context.Context, dir, reference string, opts ListOptions, yield func(path, lang string, rows []OutlineRow) bool) error {
	if yield == nil {
		return ErrWalkNilYield
	}
	ref := ParseReference(reference)
	src, _, err := listExtractSource(ctx, dir, ref, opts)
	if err != nil {
		return err
	}
	st := store.New()
	return WalkStore(ctx, src, st, func(path string) bool {
		return yield(path, FileLang(st, path), OutlineListStore(ctx, st, path, ref, opts))
	})
}

// OutlineListStore is OutlineListExtract from RelationAtom/RelationScope for one file.
func OutlineListStore(ctx context.Context, st *store.Store, fp string, ref Reference, opts ListOptions) []OutlineRow {
	if st == nil || fp == "" {
		return nil
	}
	var listed []project.AtomDef
	_ = yieldAtomsFromStore(ctx, st, ref, "", true, true, opts, func(sym AtomInfo) bool {
		p := strings.TrimPrefix(filepath.ToSlash(ParseReference(sym.Atom.Reference).Path), "./")
		want := strings.TrimPrefix(filepath.ToSlash(fp), "./")
		if p != want {
			return true
		}
		listed = append(listed, project.AtomDef{
			Name:      ParseReference(sym.Atom.Reference).Name,
			StartByte: sym.Atom.StartByte,
			EndByte:   sym.Atom.EndByte,
			ScopeIdx:  atomScopeIdx(st, fp, sym.Atom.StartByte, ParseReference(sym.Atom.Reference).Name),
		})
		return true
	})
	return OutlineFile(scopesForFile(st, fp), listed, nil)
}

func scopesForFile(st *store.Store, fp string) []project.ScopeDef {
	max := -1
	want := strings.TrimPrefix(filepath.ToSlash(fp), "./")
	match := func(p string) bool {
		return p == fp || strings.TrimPrefix(filepath.ToSlash(p), "./") == want
	}
	for _, t := range st.Rows(store.RelationScope) {
		if len(t) < 5 || !match(t[0]) {
			continue
		}
		if i := store.AtoiInt(t[1]); i > max {
			max = i
		}
	}
	if max < 0 {
		return nil
	}
	out := make([]project.ScopeDef, max+1)
	for _, t := range st.Rows(store.RelationScope) {
		if len(t) < 5 || !match(t[0]) {
			continue
		}
		out[store.AtoiInt(t[1])] = project.ScopeDef{
			Parent:    store.AtoiInt(t[2]),
			StartByte: store.Atoi(t[3]),
			EndByte:   store.Atoi(t[4]),
		}
	}
	return out
}

func atomScopeIdx(st *store.Store, fp string, start uint32, name string) int {
	want := strings.TrimPrefix(filepath.ToSlash(fp), "./")
	for _, t := range st.Rows(store.RelationAtom) {
		if len(t) < 6 {
			continue
		}
		p := strings.TrimPrefix(filepath.ToSlash(t[0]), "./")
		if p != want && t[0] != fp {
			continue
		}
		if t[1] == name && store.Atoi(t[2]) == start {
			return store.AtoiInt(t[5])
		}
	}
	return -1
}

// OutlineListExtract is the nested atoms list for one already-scoped extract
// (WalkListExtracts picked the file). Same name/export filters as WalkAtoms.
func OutlineListExtract(ctx context.Context, fe *project.FileExtract, ref Reference, opts ListOptions) []OutlineRow {
	if fe == nil {
		return nil
	}
	var listed []project.AtomDef
	yieldAtomsFromExtract(ctx, fe, ref, "", true, true, opts, func(sym AtomInfo) bool {
		idx := -1
		for _, a := range fe.Atoms {
			if a.Name == sym.Reference.Name && a.StartByte == sym.Atom.StartByte {
				idx = a.ScopeIdx
				break
			}
		}
		listed = append(listed, project.AtomDef{
			Name:      sym.Reference.Name,
			StartByte: sym.Atom.StartByte,
			EndByte:   sym.Atom.EndByte,
			ScopeIdx:  idx,
		})
		return true
	})
	mini := *fe
	mini.Atoms = listed
	mini.Imports = nil
	return OutlineFromExtract(&mini)
}

func yieldAtomsFromStore(ctx context.Context, st *store.Store, ref Reference, refPath string, refIsDir, recursive bool, opts ListOptions, yield func(AtomInfo) bool) error {
	if st == nil {
		return nil
	}
	langOf := map[string]string{}
	for _, t := range st.Rows(store.RelationFile) {
		if len(t) >= 2 {
			langOf[t[0]] = t[1]
			langOf[strings.TrimPrefix(t[0], "./")] = t[1]
		}
	}
	for _, t := range st.Rows(store.RelationAtom) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(t) < 6 {
			continue
		}
		fp, name := t[0], t[1]
		entPath := strings.TrimPrefix(filepath.ToSlash(fp), "./")
		entRef := ParseReference(AtomRef("./"+entPath, name))
		if ref.Name != "" && entRef.Name != ref.Name {
			continue
		}
		if !matchesListPathScope(entPath, refPath, refIsDir, recursive) {
			continue
		}
		ent := project.Atom{
			Reference: entRef.String(),
			StartByte: store.Atoi(t[2]),
			EndByte:   store.Atoi(t[3]),
			Exported:  t[4] == "1",
		}
		language := langOf[fp]
		if language == "" {
			language = langOf[entPath]
		}
		if !providerAllowListAtom(ctx, ref, entRef, entPath, language, opts) {
			continue
		}
		if !allowListedAtom(ent, opts.IncludeHidden) {
			continue
		}
		out := AtomInfo{Atom: ent, Reference: entRef, Language: language}
		out.Reference = providerListOutputReference(ref, entRef)
		out.Atom.Reference = out.Reference.String()
		if !yield(out) {
			return nil
		}
	}
	return nil
}

// yieldAtomsFromExtract returns false if the caller should stop WalkExtracts.
func yieldAtomsFromExtract(ctx context.Context, fe *project.FileExtract, ref Reference, refPath string, refIsDir, recursive bool, opts ListOptions, yield func(AtomInfo) bool) bool {
	if fe == nil {
		return true
	}
	for _, entDef := range fe.Atoms {
		entPath := strings.TrimPrefix(filepath.ToSlash(fe.Path), "./")
		entRef := ParseReference(AtomRef("./"+entPath, entDef.Name))

		if ref.Name != "" && entRef.Name != ref.Name {
			continue
		}
		if !matchesListPathScope(entPath, refPath, refIsDir, recursive) {
			continue
		}

		ent := project.Atom{
			Reference: entRef.String(),
			StartByte: entDef.StartByte,
			EndByte:   entDef.EndByte,
			Exported:  entDef.Exported,
		}
		language := fe.Language
		if !providerAllowListAtom(ctx, ref, entRef, entPath, language, opts) {
			continue
		}
		if !allowListedAtom(ent, opts.IncludeHidden) {
			continue
		}

		out := AtomInfo{
			Atom:      ent,
			Reference: entRef,
			Language:  language,
		}
		out.Reference = providerListOutputReference(ref, entRef)
		out.Atom.Reference = out.Reference.String()

		if !yield(out) {
			return false
		}
	}
	return true
}

func listScopeForRef(dir string, ref Reference) (refPath string, refIsDir bool) {
	if ref.Provider != "path" {
		return "", false
	}

	refPath = strings.TrimPrefix(ref.Path, "./")
	if refPath == "." {
		refPath = ""
	}

	absPath := ref.Path
	if !filepath.IsAbs(absPath) {
		absPath = lewpath.New(dir, refPath).String()
	}
	if st, err := (projectfs.OS{}).Stat(absPath); err == nil && st.IsDir() {
		refIsDir = true
	}
	return refPath, refIsDir
}

func matchesListPathScope(entPath, refPath string, refIsDir, recursive bool) bool {
	entPath = filepath.ToSlash(entPath)
	refPath = filepath.ToSlash(refPath)
	refPath = strings.TrimPrefix(refPath, "./")
	if refPath == "." {
		refPath = ""
	}

	if refIsDir {
		if recursive {
			if refPath == "" {
				return true
			}
			return entPath == refPath || strings.HasPrefix(entPath, refPath+"/")
		}
		parent := filepath.ToSlash(path.Dir(entPath))
		if parent == "." {
			parent = ""
		}
		return parent == refPath
	}

	if refPath == "" {
		return true
	}
	return entPath == refPath
}

func allowListedAtom(ent project.Atom, includeHidden bool) bool {
	return includeHidden || ent.Exported
}
