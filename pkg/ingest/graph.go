package ingest

import (
	"context"
	"path"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/datalog"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/projectfs"
	"github.com/lewtec/patlint/pkg/store"
)

// evalGraph is Materialize stratum 1–2: finder bags → store → Datalog → Result.
func evalGraph(ctx context.Context, rootDir string, extracts []*project.FileExtract, policy PackQueries) (*project.Result, error) {
	st, host, fams := loadStore(rootDir, extracts, policy)
	if err := datalog.Eval(ctx, st, datalog.ResolveProgram(), host); err != nil {
		return nil, err
	}
	store.FillBinds(st)
	return store.Project(st, fams), nil
}

// FileEffects is strata 1–3 for extracts: binds close, then finding/edit.
func FileEffects(ctx context.Context, rootDir string, extracts []*project.FileExtract, policy PackQueries) (*store.Store, error) {
	st, host, _ := loadStore(rootDir, extracts, policy)
	if err := datalog.Eval(ctx, st, datalog.ResolveProgram(), host); err != nil {
		return nil, err
	}
	store.FillBinds(st)
	if err := datalog.Eval(ctx, st, datalog.QueryProgram(), host); err != nil {
		return nil, err
	}
	return st, nil
}

func loadStore(rootDir string, extracts []*project.FileExtract, policy PackQueries) (*store.Store, datalog.StoreHost, []project.FamilyClaim) {
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		rootAbs = rootDir
	}
	st := store.New()
	fams := familiesOf(policy)
	store.LoadIngest(st, extracts, fams, ingestRules(policy))
	files, dirs := knownPaths(extracts)
	host := datalog.StoreHost{
		Store:      st,
		Root:       rootAbs,
		KnownFiles: files,
		KnownDirs:  dirs,
		ResolveImport: func(spec, importer, root string, kf, kd map[string]bool) string {
			if policy == nil {
				return ""
			}
			return policy.ResolveImport(spec, ImportResolveContext{
				RootDir:      root,
				ImporterPath: strings.TrimPrefix(importer, "./"),
				KnownFiles:   kf,
				KnownDirs:    kd,
				Representant: policy,
			})
		},
	}
	return st, host, fams
}

func knownPaths(extracts []*project.FileExtract) (files, dirs map[string]bool) {
	files = map[string]bool{}
	dirs = map[string]bool{}
	for _, fe := range extracts {
		if fe == nil {
			continue
		}
		p := strings.TrimPrefix(filepath.ToSlash(fe.Path), "./")
		files[p] = true
		dir := path.Dir(p)
		for dir != "." && dir != "/" && dir != "" {
			dirs[dir] = true
			parent := path.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		if i := strings.IndexByte(p, '/'); i > 0 {
			dirs[p[:i]] = true
		}
	}
	return files, dirs
}

func knownPathsFromStore(st *store.Store) (files, dirs map[string]bool) {
	files = map[string]bool{}
	dirs = map[string]bool{}
	if st == nil {
		return files, dirs
	}
	for _, t := range st.Rows(store.RelationFile) {
		if len(t) < 1 {
			continue
		}
		p := strings.TrimPrefix(filepath.ToSlash(t[0]), "./")
		files[p] = true
		dir := path.Dir(p)
		for dir != "." && dir != "/" && dir != "" {
			dirs[dir] = true
			parent := path.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		if i := strings.IndexByte(p, '/'); i > 0 {
			dirs[p[:i]] = true
		}
	}
	return files, dirs
}

// FileLang is the RelationFile language for path, or "".
func FileLang(st *store.Store, path string) string {
	if st == nil || path == "" {
		return ""
	}
	want := strings.TrimPrefix(filepath.ToSlash(path), "./")
	for _, t := range st.Rows(store.RelationFile) {
		if len(t) < 2 {
			continue
		}
		if t[0] == path || strings.TrimPrefix(t[0], "./") == want {
			return t[1]
		}
	}
	return ""
}

// Ingest writes extract views into st (stratum 1).
func Ingest(st *store.Store, extracts []*project.FileExtract, policy PackQueries) {
	store.LoadIngest(st, extracts, familiesOf(policy), ingestRules(policy))
}

// EvalStore is strata 1–2 on an already-loaded store.
func EvalStore(ctx context.Context, rootDir string, st *store.Store, policy PackQueries) (*project.Result, error) {
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		rootAbs = rootDir
	}
	files, dirs := knownPathsFromStore(st)
	host := datalog.StoreHost{
		Store:      st,
		Root:       rootAbs,
		KnownFiles: files,
		KnownDirs:  dirs,
		ResolveImport: func(spec, importer, root string, kf, kd map[string]bool) string {
			if policy == nil {
				return ""
			}
			return policy.ResolveImport(spec, ImportResolveContext{
				RootDir:      root,
				ImporterPath: strings.TrimPrefix(importer, "./"),
				KnownFiles:   kf,
				KnownDirs:    kd,
				Representant: policy,
			})
		},
	}
	if err := datalog.Eval(ctx, st, datalog.ResolveProgram(), host); err != nil {
		return nil, err
	}
	store.FillBinds(st)
	return store.Project(st, familiesOf(policy)), nil
}

func writeLangFlags(st *store.Store, policy PackQueries, lang string) {
	if st == nil || lang == "" {
		return
	}
	r := languageRules(policy, lang)
	if r.PackageScopedBareNames {
		st.Insert(store.RelationLanguage, store.Tuple{lang, store.FlagPackageScoped})
	}
	if r.EmptyPackageDirScoped {
		st.Insert(store.RelationLanguage, store.Tuple{lang, store.FlagEmptyDir})
	}
	if r.NestedTypeMembers {
		st.Insert(store.RelationLanguage, store.Tuple{lang, store.FlagNested})
	}
	if r.IncludeFileExportsBare {
		st.Insert(store.RelationLanguage, store.Tuple{lang, store.FlagIncludeExport})
	}
}

func ingestRules(policy PackQueries) func(string) project.LanguageRules {
	return func(lang string) project.LanguageRules {
		return languageRules(policy, lang)
	}
}

// ExpandImportsInto is one-hop import/reexport targets under root (not recursive).
func ExpandImportsInto(ctx context.Context, st *store.Store, sess *project.Session, rootAbs string, fsys projectfs.FS, policy PackQueries) error {
	if st == nil || sess == nil || policy == nil {
		return nil
	}
	if fsys == nil {
		fsys = projectfs.OS{}
	}
	known, _ := knownPathsFromStore(st)
	seen := map[string]bool{}
	for p := range known {
		seen[lewpath.New(rootAbs, filepath.FromSlash(p)).String()] = true
	}
	var files []string
	for _, t := range st.Rows(store.RelationFile) {
		if len(t) > 0 {
			files = append(files, t[0])
		}
	}
	for _, fp := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		imp := ImportResolveContext{
			RootDir:      rootAbs,
			ImporterPath: fp,
			KnownFiles:   known,
			Representant: policy,
		}
		for _, spec := range importSpecs(st, fp) {
			refStr := policy.ResolveImport(spec, imp)
			if refStr == "" {
				continue
			}
			r := ParseReference(refStr)
			if r.Provider != "path" || r.Path == "" {
				continue
			}
			rel := strings.TrimPrefix(filepath.ToSlash(r.Path), "./")
			abs := lewpath.New(rootAbs, filepath.FromSlash(rel)).String()
			if seen[abs] {
				continue
			}
			seen[abs] = true
			extra, err := parseFileAt(ctx, sess, rootAbs, abs, nil, fsys, policy, st)
			if err != nil || extra == nil {
				continue
			}
			known[strings.TrimPrefix(filepath.ToSlash(extra.Path), "./")] = true
		}
	}
	return nil
}

func importSpecs(st *store.Store, fp string) []string {
	var out []string
	for _, t := range st.Rows(store.RelationImport) {
		if len(t) >= 3 && (t[0] == fp || pathDotStore(t[0]) == pathDotStore(fp)) && t[2] != "" {
			out = append(out, t[2])
		}
	}
	for _, t := range st.Rows(store.RelationReexport) {
		if len(t) >= 4 && (t[0] == fp || pathDotStore(t[0]) == pathDotStore(fp)) && t[3] != "" {
			out = append(out, t[3])
		}
	}
	return out
}

func pathDotStore(rel string) string {
	if strings.HasPrefix(rel, "./") || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
		return rel
	}
	return "./" + rel
}
