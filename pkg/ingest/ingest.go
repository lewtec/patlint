package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingestutil"

	"github.com/lewtec/patlint/pkg/ignore"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/projectfs"
	"github.com/lewtec/patlint/pkg/store"
)

// appendImportTargetExtracts parses files referenced by imports/reexports when
// the VM resolves them to a path under rootAbs. Does not recurse into
// dependency trees.
// sess must be non-nil (caller-owned Session).
func appendImportTargetExtracts(ctx context.Context, sess *project.Session, rootAbs string, extracts []*project.FileExtract, seen map[string]bool, fsys projectfs.FS, policy PackQueries) []*project.FileExtract {
	if sess == nil {
		return extracts
	}
	if seen == nil {
		seen = map[string]bool{}
	}
	if fsys == nil {
		fsys = projectfs.OS{}
	}
	known := map[string]bool{}
	for _, fe := range extracts {
		if fe == nil {
			continue
		}
		p := strings.TrimPrefix(filepath.ToSlash(fe.Path), "./")
		known[p] = true
	}
	n := len(extracts)
	for i := 0; i < n; i++ {
		if err := ctx.Err(); err != nil {
			return extracts
		}
		fe := extracts[i]
		if fe == nil {
			continue
		}
		var specs []string
		for _, im := range fe.Imports {
			specs = append(specs, im.SourcePath)
		}
		for _, re := range fe.Reexports {
			specs = append(specs, re.SourcePath)
		}
		if len(specs) == 0 {
			continue
		}
		imp := ImportResolveContext{
			RootDir:      rootAbs,
			ImporterPath: fe.Path,
			KnownFiles:   known,
			Representant: policy,
		}
		if policy == nil {
			continue
		}
		for _, spec := range specs {
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
			extra, err := parseFileAt(ctx, sess, rootAbs, abs, nil, fsys, policy, nil)
			if err != nil || extra == nil {
				continue
			}
			extracts = append(extracts, extra)
			known[strings.TrimPrefix(filepath.ToSlash(extra.Path), "./")] = true
		}
	}
	return extracts
}

// bfsNeighbors returns absolute paths worth extracting next for inspection BFS:
// other source files in the same directory (same-package / co-located), and
// local files/dirs suggested by import specs (filesystem probe under root).
// eng filters co-located peers (linguist-generated, refactree-ignored, etc.); nil explores all.
func bfsNeighbors(sess *project.Session, rootAbs string, fe *project.FileExtract, eng *ignore.Engine, policy PackQueries) []string {
	var out []string
	absDir := lewpath.New(rootAbs, filepath.FromSlash(filepath.Dir(fe.Path))).String()
	if fe.Path == "." || filepath.Dir(fe.Path) == "." {
		absDir = rootAbs
	}
	// Co-located sources (Go package peers, sibling modules, etc.).
	if !isVendoredPath(rootAbs, absDir) {
		out = append(out, listSourceFilesInDir(sess, absDir, eng, policy)...)
	}

	importerDir := filepath.ToSlash(filepath.Dir(fe.Path))
	if importerDir == "." {
		importerDir = ""
	}
	for _, imp := range fe.Imports {
		out = append(out, probeImportTargets(sess, rootAbs, importerDir, imp.SourcePath, policy)...)
	}
	for _, re := range fe.Reexports {
		out = append(out, probeImportTargets(sess, rootAbs, importerDir, re.SourcePath, policy)...)
	}
	return out
}

func listSourceFilesInDir(sess *project.Session, absDir string, eng *ignore.Engine, policy PackQueries) []string {
	if isSkippedDirName(filepath.Base(absDir)) {
		return nil
	}
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if _, ok, err := attributeHost(policy, name); err != nil || !ok {
			continue
		}
		abs := lewpath.New(absDir, name).String()
		if eng != nil && !eng.CheckPath(abs, false).Explore {
			continue
		}
		out = append(out, abs)
	}
	return out
}

// ListSourceFilesInDir returns absolute paths of recognized language sources
// directly under absDir (non-recursive). eng nil explores all; otherwise only
// paths with Explore from ignore.Engine. Shared by Seed BFS peers and package hops.
func ListSourceFilesInDir(sess *project.Session, absDir string, eng *ignore.Engine, policy PackQueries) []string {
	return listSourceFilesInDir(sess, absDir, eng, policy)
}

// PackageSourceFiles returns absolute hop targets for a package visit.
// Dir: ignore-aware peers in abs. File + dirModule: peers in the parent dir.
// Other files: only abs (even when ignore would skip — the caller named the path).
// dirModule is Walker/VM DirectoryModule for this file (not a Session lookup).
//
// ignore.Collect failure degrades to eng=nil so a bad attrs file does not block hops.
func PackageSourceFiles(ctx context.Context, sess *project.Session, abs string, isDir, dirModule bool, policy PackQueries) []string {
	if sess == nil {
		if isDir {
			return listSourceFilesInDir(nil, abs, nil, policy)
		}
		return []string{abs}
	}
	var eng *ignore.Engine
	if sess.Root != "" {
		if e, err := ignore.Collect(ctx, sess.Root); err == nil {
			eng = e
		}
	}
	if isDir {
		return listSourceFilesInDir(sess, abs, eng, policy)
	}
	if dirModule {
		if peers := listSourceFilesInDir(sess, filepath.Dir(abs), eng, policy); len(peers) > 0 {
			return peers
		}
	}
	return []string{abs}
}

// IsSkippedDirName reports dependency/build trees that must not be walked for
// inspection BFS, full ingest, or project-wide grep/rewrite (still parse a file
// if it is an explicit seed path). Delegates to pkg/ignore builtins.
func IsSkippedDirName(name string) bool {
	return ignore.IsSkippedDirName(name)
}

// isSkippedDirName is the historical unexported name; keep call sites working.
func isSkippedDirName(name string) bool { return IsSkippedDirName(name) }

// isVendoredPath reports whether abs is under a skipped dependency/build dir
// relative to rootAbs (or absolute path containing those segments).
func isVendoredPath(rootAbs, abs string) bool {
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		rel = abs
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if isSkippedDirName(part) {
			return true
		}
	}
	return false
}

// probeImportTargets maps an import spec to local files/dirs under rootAbs
// without requiring a prior full walk (knownFiles may be incomplete during BFS).
func probeImportTargets(sess *project.Session, rootAbs, importerDirRel, sourcePath string, policy PackQueries) []string {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return nil
	}

	var candidates []string

	// Relative path (JS/Python relative, or ./foo).
	if strings.HasPrefix(sourcePath, ".") || strings.HasPrefix(sourcePath, "/") {
		base := importerDirRel
		if strings.HasPrefix(sourcePath, "/") {
			base = ""
		}
		joined := filepath.ToSlash(filepath.Clean(lewpath.New(base, sourcePath).String()))
		joined = strings.TrimPrefix(joined, "./")
		candidates = append(candidates, joined)
	}

	// Dotted / slashed module path (Python absolute, some JS).
	mod := strings.ReplaceAll(sourcePath, ".", "/")
	candidates = append(candidates, mod, sourcePath)

	// Go-style first segment as local dir (when not a full module URL).
	if !strings.Contains(sourcePath, ".") || strings.Count(sourcePath, "/") > 0 {
		candidates = append(candidates, sourcePath)
	}
	if i := strings.Index(sourcePath, "/"); i > 0 {
		candidates = append(candidates, sourcePath[:i])
	}

	seen := map[string]bool{}
	var out []string
	var add func(abs string)
	var addPackRepresentants func(abs, rel string)
	add = func(abs string) {
		if abs == "" || seen[abs] {
			return
		}
		st, err := os.Stat(abs)
		if err != nil {
			return
		}
		seen[abs] = true
		if st.IsDir() {
			// Never expand node_modules / vendor package roots as flat file lists.
			if isSkippedDirName(filepath.Base(abs)) {
				return
			}
			if isVendoredPath(rootAbs, abs) {
				rel, err := filepath.Rel(rootAbs, abs)
				if err != nil {
					return
				}
				addPackRepresentants(abs, filepath.ToSlash(rel))
				return
			}
			// Import probes have no engine: resolution is intentional, not crawl.
			out = append(out, listSourceFilesInDir(sess, abs, nil, policy)...)
			return
		}
		if _, ok, err := attributeHost(policy, abs); err == nil && ok {
			out = append(out, abs)
		}
	}
	addPackRepresentants = func(abs, rel string) {
		rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
		for _, g := range policy.DirectoryRepresentants() {
			name := representantBase(g)
			if name == "" {
				continue
			}
			childRel := name
			if rel != "" && rel != "." {
				childRel = path.Join(rel, name)
			}
			if !policy.MatchDirectoryRepresentant(childRel) {
				continue
			}
			add(lewpath.New(abs, filepath.FromSlash(name)).String())
		}
	}

	for _, c := range candidates {
		c = strings.Trim(strings.TrimPrefix(filepath.ToSlash(c), "./"), "/")
		if c == "" || c == ".." || strings.HasPrefix(c, "../") {
			continue
		}
		// Bare package names (no relative path) that only resolve under node_modules
		// are left symbolic — BFS must not enter dependency trees from project code.
		// Relative hops (./foo) under an already-vendored seed remain allowed via abs paths.
		base := lewpath.New(rootAbs, filepath.FromSlash(c)).String()
		if !strings.HasPrefix(sourcePath, ".") && isVendoredPath(rootAbs, base) {
			continue
		}
		add(base)
		add(base + ".py")
		add(base + ".go")
		add(base + ".js")
		addPackRepresentants(base, c)
	}
	return out
}

// parseFile parses a single source file and returns its FileExtract.
// Returns nil (no error) for unsupported file types.
//
// grammar.Parser serializes its own native handle; callers need no external
// parse lock (prefer one Parser per goroutine under parallel load).
//
// Some pure-Go tree-sitter grammars can SIGSEGV on valid source (observed on
// Go slice/variadic patterns). SetPanicOnFault turns that into a recoverable
// error so HTTP serve and ingest walks stay up.
func parseFile(ctx context.Context, sess *project.Session, dir, absPath string, fsys projectfs.FS, policy PackQueries, into *store.Store) (*project.FileExtract, error) {
	if sess == nil {
		return nil, ErrNilSession
	}
	if fsys == nil {
		fsys = sessionProjectFS(sess)
	}

	relPath, err := filepath.Rel(dir, absPath)
	if err != nil {
		return nil, err
	}
	relSlash := filepath.ToSlash(relPath)

	source, err := fsys.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", relPath, err)
	}

	hostLang, claimed, err := attributeHost(policy, relSlash)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, nil
	}

	prevFault := debug.SetPanicOnFault(true)
	defer debug.SetPanicOnFault(prevFault)

	var fe *project.FileExtract
	var parseErr error
	var parseDur, extractDur time.Duration
	var langName string
	start := time.Now()

	func() {
		defer func() {
			if r := recover(); r != nil {
				parseErr = fmt.Errorf("%w parsing %s: %v", ErrTreeSitterFault, relPath, r)
			}
		}()

		gid := GrammarID(policy, hostLang)
		if gerr := ingestutil.RequireGrammar(sess.Engine(), gid); gerr != nil {
			parseErr = fmt.Errorf("%s: %w", relPath, gerr)
			return
		}
		langName = hostLang
		slog.Debug("parseFile start",
			"path", relPath,
			"lang", langName,
			"bytes", len(source),
			"attrib", "policy",
		)
		tParse := time.Now()
		pf, err := ingestutil.ParseSource(sess.Engine(), source, relPath, gid)
		parseDur = time.Since(tParse)
		if err != nil {
			parseErr = err
			return
		}
		defer pf.Close()
		tExt := time.Now()
		if into != nil {
			err = extractIntoStore(ctx, policy, sess, into, hostLang, pf.Root, source, relSlash)
			if err != nil {
				parseErr = err
				return
			}
			fe = store.ProjectExtract(into, relSlash)
			if fe == nil {
				fe = store.ProjectExtract(into, relPath)
			}
		} else {
			fe, err = extractFile(ctx, policy, sess, hostLang, pf.Root, source, relPath)
			if err != nil {
				parseErr = err
				return
			}
		}
		extractDur = time.Since(tExt)
	}()

	total := time.Since(start)
	if parseErr != nil {
		slog.Debug("parseFile error",
			"path", relPath,
			"lang", langName,
			"err", parseErr,
			"parse_dur", parseDur,
			"total_dur", total,
		)
		return fe, parseErr
	}
	attrs := []any{
		"path", relPath,
		"lang", langName,
		"bytes", len(source),
		"parse_dur", parseDur,
		"extract_dur", extractDur,
		"total_dur", total,
	}
	if fe != nil {
		attrs = append(attrs,
			"atoms", len(fe.Atoms),
			"usages", len(fe.Usages),
			"imports", len(fe.Imports),
		)
	}
	// Slow files surface without -v so hangs are easier to spot.
	if total >= 2*time.Second {
		slog.Warn("parseFile slow", attrs...)
	} else {
		slog.Debug("parseFile done", attrs...)
	}
	return fe, parseErr
}
