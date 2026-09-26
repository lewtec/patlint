// Discovery and graph materialization for package ingest.
//
// Call sites should use:
//   - *Session for the project view (required; never invent on nil)
//   - walkExtracts / WalkAtoms for lazy listing (no graph)
//   - Source* builders + Session.Load for a closed *Result
//   - Thin aliases (ProjectResult / SeedResult / HopResult) for the common product cases
//
// Do not reintroduce a second WalkDir+parse path beside WalkExtracts.
// Do not reintroduce a process-global parse cache — bind cache to *Session.

package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ignore"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/projectfs"
	"github.com/lewtec/patlint/pkg/store"
)

// ExtractKind selects how paths enter WalkExtracts.
type ExtractKind int

const (
	// ExtractDir walks a directory tree (skip rules + recursive policy).
	ExtractDir ExtractKind = iota
	// ExtractSeed BFS-expands from seed file paths (neighbors + import probes).
	// Used for serve annotate and canonicalize file hops.
	ExtractSeed
	// ExtractHop parses only the given path(s) with no neighbor BFS.
	// Used for single-file list scope.
	ExtractHop
)

// ExtractSource is the input to WalkExtracts: which paths to parse under Root.
// Prefer SourceProject / SourceDir / SourceSeed / SourceHop over hand-building.
type ExtractSource struct {
	Kind ExtractKind
	// Root is the ingest root for path relativity, the extract cache, and Resolve.
	// Empty means ".".
	Root string

	// Dir is the directory to walk when Kind is ExtractDir.
	// Empty means Root. Paths in FileExtract are relative to Root.
	Dir string
	// Recursive controls directory descent for ExtractDir.
	Recursive bool

	// Paths are seed/hop files (absolute or relative to Root).
	// Used by ExtractSeed and ExtractHop.
	Paths []string

	// FS is optional project content (nil = disk). LSP overlays pass a projectfs.Overlay.
	FS projectfs.FS

	// Session is required. Nil → ErrNilSession.
	// Walker.Load / WalkExtracts set this.
	Session *project.Session

	// Policy attributes paths and extracts. Nil claims nothing.
	// Walker sets this to the LispVM.
	Policy PackQueries
}

// WithFS returns a copy of src with FS set (overlay / staged content).
func (src ExtractSource) WithFS(fsys projectfs.FS) ExtractSource {
	src.FS = fsys
	return src
}

// SourceProject is a full-tree Dir walk under root (mv / full-graph jobs).
func SourceProject(root string) ExtractSource {
	return ExtractSource{Kind: ExtractDir, Root: root, Recursive: true}
}

// SourceDir is a Dir walk. dir empty means root; recursive controls descent.
// Use for package scopes and provider package trees.
func SourceDir(root, dir string, recursive bool) ExtractSource {
	return ExtractSource{Kind: ExtractDir, Root: root, Dir: dir, Recursive: recursive}
}

// SourceSeed is Seed BFS from one or more paths (neighbors + import probes).
func SourceSeed(root string, paths ...string) ExtractSource {
	return ExtractSource{Kind: ExtractSeed, Root: root, Paths: paths}
}

// SourceHop parses only the named paths (no neighbor BFS, no ignore skip).
func SourceHop(root string, paths ...string) ExtractSource {
	return ExtractSource{Kind: ExtractHop, Root: root, Paths: paths}
}

// MaterializeOptions controls graph construction over a closed extract set.
type MaterializeOptions struct {
	// ExpandImports one-hop parses import/reexport targets under Root.
	// True only for project-complete / mv-style loads (SPEC). Default false.
	ExpandImports bool
	// FS is optional project content for ExpandImports reads (nil = disk).
	// Load copies src.FS here when this field is nil.
	FS projectfs.FS
	// Session is required when ExpandImports is true (and for Load).
	// Session.Load / Walker.Load set this from the receiver.
	Session *project.Session
	// Policy attributes ExpandImports hops. Nil attributes nothing. Walker.Load sets this.
	Policy PackQueries
}

// WalkExtracts pulls FileExtract values for src, invoking yield for each
// non-nil extract. Returning false from yield stops early (lazy list / fzf).
// ctx is checked between files (not mid-parse); cancel avoids starting the next
// parse. Nil ctx is rejected (callers must pass parent context, e.g. cmd.Context()).
// It never resolves the graph and never one-hop expands imports (Seed may BFS).
// src.Session is required.
func WalkExtracts(ctx context.Context, src ExtractSource, yield func(*project.FileExtract) bool) error {
	return walkExtractsInto(ctx, src, nil, yield)
}

func walkExtractsInto(ctx context.Context, src ExtractSource, st *store.Store, yield func(*project.FileExtract) bool) error {
	if yield == nil {
		return ErrWalkNilYield
	}
	if src.Session == nil {
		return ErrNilSession
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	rootAbs, err := absRoot(src.Root)
	if err != nil {
		return err
	}

	fsys := src.FS
	if fsys == nil {
		fsys = sessionProjectFS(src.Session)
	}
	sess := src.Session

	switch src.Kind {
	case ExtractDir:
		dirAbs := rootAbs
		if src.Dir != "" {
			if filepath.IsAbs(src.Dir) {
				dirAbs, err = filepath.Abs(src.Dir)
			} else {
				dirAbs, err = filepath.Abs(lewpath.New(rootAbs, src.Dir).String())
			}
			if err != nil {
				return err
			}
		}
		return walkExtractsDir(ctx, sess, rootAbs, dirAbs, src.Recursive, fsys, src.Policy, yield, st)

	case ExtractSeed:
		paths, err := normalizeSourcePaths(rootAbs, src.Paths)
		if err != nil {
			return err
		}
		return walkExtractsSeed(ctx, sess, rootAbs, paths, fsys, src.Policy, yield, st)

	case ExtractHop:
		paths, err := normalizeSourcePaths(rootAbs, src.Paths)
		if err != nil {
			return err
		}
		return walkExtractsHop(ctx, sess, rootAbs, paths, fsys, src.Policy, yield, st)

	default:
		return fmt.Errorf("%w %d", ErrWalkUnknownKind, src.Kind)
	}
}

// CollectExtracts drains WalkExtracts into a slice (complete materialize input).
// src.Session is required.
func CollectExtracts(ctx context.Context, src ExtractSource) ([]*project.FileExtract, error) {
	var out []*project.FileExtract
	err := WalkExtracts(ctx, src, func(fe *project.FileExtract) bool {
		out = append(out, fe)
		return true
	})
	return out, err
}

// WalkStore parses each file into st and yields its path. yield may be nil.
func WalkStore(ctx context.Context, src ExtractSource, st *store.Store, yield func(path string) bool) error {
	if st == nil {
		return fmt.Errorf("WalkStore: nil store")
	}
	out := yield
	if out == nil {
		out = func(string) bool { return true }
	}
	return walkExtractsInto(ctx, src, st, func(fe *project.FileExtract) bool {
		if fe == nil {
			return true
		}
		return out(fe.Path)
	})
}

// MaterializeStore walks files into one store and evaluates strata 1–2.
// Store is truth. MaterializeSource is the Result projection of this.
func MaterializeStore(ctx context.Context, src ExtractSource, opts MaterializeOptions) (*store.Store, error) {
	sess := opts.Session
	if sess == nil {
		sess = src.Session
	}
	if sess == nil {
		return nil, ErrNilSession
	}
	src.Session = sess
	opts.Session = sess
	if opts.Policy == nil {
		opts.Policy = src.Policy
	}
	if src.Policy == nil {
		src.Policy = opts.Policy
	}
	root := src.Root
	if root == "" {
		root = "."
	}
	if opts.FS == nil {
		opts.FS = src.FS
	}
	rootAbs, err := absRoot(root)
	if err != nil {
		rootAbs = root
	}
	st := store.New()
	if err := WalkStore(ctx, src, st, nil); err != nil {
		return nil, err
	}
	if opts.ExpandImports {
		if err := ExpandImportsInto(ctx, st, sess, rootAbs, opts.FS, opts.Policy); err != nil {
			return nil, err
		}
	}
	if _, err := EvalStore(ctx, rootAbs, st, opts.Policy); err != nil {
		return nil, err
	}
	return st, nil
}

// MaterializeSource is MaterializeStore projected to Result.
func MaterializeSource(ctx context.Context, src ExtractSource, opts MaterializeOptions) (*project.Result, error) {
	st, err := MaterializeStore(ctx, src, opts)
	if err != nil {
		return nil, err
	}
	return store.Project(st, familiesOf(opts.Policy)), nil
}

func absRoot(root string) (string, error) {
	if root == "" {
		root = "."
	}
	return filepath.Abs(root)
}

func normalizeSourcePaths(rootAbs string, paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		var abs string
		if filepath.IsAbs(p) {
			abs = p
		} else {
			// Prefer cwd-absolute when the path already resolves under root
			// (e.g. join(relRoot, file) from WalkAtoms). Else join to rootAbs.
			if cand, err := filepath.Abs(p); err == nil {
				if rel, err := filepath.Rel(rootAbs, cand); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					abs = cand
				}
			}
			if abs == "" {
				abs = lewpath.New(rootAbs, filepath.FromSlash(strings.TrimPrefix(p, "./"))).String()
			}
		}
		abs, err := filepath.Abs(abs)
		if err != nil {
			return nil, err
		}
		out = append(out, abs)
	}
	return out, nil
}

func walkExtractsDir(ctx context.Context, sess *project.Session, parseRoot, dirAbs string, recursive bool, fsys projectfs.FS, policy PackQueries, yield func(*project.FileExtract) bool, st *store.Store) error {
	slog.Debug("extract walk start", "root", parseRoot, "dir", dirAbs, "recursive", recursive)
	eng, err := ignore.Collect(ctx, parseRoot)
	if err != nil {
		return err
	}
	visited := 0
	skipped := 0
	err = filepath.WalkDir(dirAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			// Enter walk root even if it would be ignored as a basename elsewhere;
			// SkipDir still applies to children (node_modules, linguist trees, …).
			if path != dirAbs && eng.SkipDir(path) {
				slog.Debug("extract skip dir", "path", path, "name", d.Name())
				return filepath.SkipDir
			}
			if !recursive && path != dirAbs {
				return filepath.SkipDir
			}
			return nil
		}
		// Dir crawl respects ignore.Engine (builtins + gitattributes attrs).
		// ExtractHop / explicit seed paths intentionally do not filter here.
		if !eng.CheckPath(path, false).Explore {
			skipped++
			slog.Debug("extract skip ignored", "path", path)
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		// Atomic per file: cancel is observed before parse, not mid tree-sitter.
		if err := ctx.Err(); err != nil {
			return err
		}
		fe, parseErr := parseFileAt(ctx, sess, parseRoot, path, info, fsys, policy, st)
		if parseErr != nil {
			return parseErr
		}
		if fe == nil {
			skipped++
			return nil
		}
		visited++
		slog.Debug("extract visit", "path", fe.Path, "lang", fe.Language)
		if !yield(fe) {
			return filepath.SkipAll
		}
		return nil
	})
	slog.Debug("extract walk done", "dir", dirAbs, "visited", visited, "skipped_no_lang", skipped, "err", err)
	return err
}

func walkExtractsHop(ctx context.Context, sess *project.Session, rootAbs string, paths []string, fsys projectfs.FS, policy PackQueries, yield func(*project.FileExtract) bool, st *store.Store) error {
	for _, abs := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		fe, err := parseFileAt(ctx, sess, rootAbs, abs, nil, fsys, policy, st)
		if err != nil {
			return err
		}
		if fe == nil {
			continue
		}
		if !yield(fe) {
			return nil
		}
	}
	return nil
}

func walkExtractsSeed(ctx context.Context, sess *project.Session, rootAbs string, seeds []string, fsys projectfs.FS, policy PackQueries, yield func(*project.FileExtract) bool, st *store.Store) error {
	eng, err := ignore.Collect(ctx, rootAbs)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	queue := append([]string(nil), seeds...)

	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		absPath := queue[0]
		queue = queue[1:]
		if seen[absPath] {
			continue
		}
		seen[absPath] = true

		if rel, err := filepath.Rel(rootAbs, absPath); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}

		// Explicit seeds still parse (open this file). Neighbors use Explore below.
		fe, err := parseFileAt(ctx, sess, rootAbs, absPath, nil, fsys, policy, st)
		if err != nil {
			return err
		}
		if fe == nil {
			continue
		}
		if !yield(fe) {
			return nil
		}

		if isVendoredPath(rootAbs, absPath) {
			continue
		}
		for _, neigh := range bfsNeighbors(sess, rootAbs, fe, eng, policy) {
			if !seen[neigh] {
				queue = append(queue, neigh)
			}
		}
	}
	return nil
}
