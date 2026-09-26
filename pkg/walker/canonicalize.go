package walker

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/lewtec/patlint/pkg/ingest"
)

// CanonicalizeReference turns a reference into the preferred form for navigation.
// Directory prelude then hop/dir Load + CanonicalizeInResult. Missing hops
// return the best ref so far (no error).
func (w *Walker) CanonicalizeReference(ctx context.Context, rootDir string, ref ingest.Reference) ingest.Reference {
	if w == nil || w.Sess == nil || ctx == nil {
		return ref
	}
	ref = ingest.NormalizePathReference(ref)
	if ref.Provider == "" {
		ref.Provider = "path"
	}
	origProvider := ref.Provider
	origPath := ref.Path

	rootAbs, err := filepath.Abs(rootDir)
	if err != nil || rootAbs == "" {
		rootAbs = rootDir
		if rootAbs == "" {
			rootAbs = "."
		}
	}

	if ref.Provider == "path" {
		ref = ingest.CanonicalizePathReference(w.VM, rootAbs, ref)
	}

	const maxOuter = 16
	for hop := 0; hop < maxOuter; hop++ {
		if err := ctx.Err(); err != nil {
			return ingest.ProjectToInputProvider(origProvider, origPath, ref)
		}
		result, ok := w.loadForCanonicalize(ctx, rootAbs, ref)
		if !ok || result == nil {
			return ingest.ProjectToInputProvider(origProvider, origPath, ref)
		}
		next := ingest.CanonicalizeInResult(result, ref)
		if next.String() == ref.String() {
			return ingest.ProjectToInputProvider(origProvider, origPath, next)
		}
		if ingest.SameScopePath(ref, next) {
			return ingest.ProjectToInputProvider(origProvider, origPath, next)
		}
		ref = next
	}
	return ingest.ProjectToInputProvider(origProvider, origPath, ref)
}

func (w *Walker) loadForCanonicalize(ctx context.Context, rootAbs string, ref ingest.Reference) (*project.Result, bool) {
	if w == nil || ctx == nil {
		return nil, false
	}
	if ref.Provider == "" || ref.Provider == "path" {
		rel := strings.TrimPrefix(ref.Path, "./")
		if rel == "" || rel == "." {
			return nil, false
		}
		abs := ref.Path
		if !filepath.IsAbs(abs) {
			abs = lewpath.New(rootAbs, filepath.FromSlash(rel)).String()
		}
		st, err := os.Stat(abs)
		if err != nil {
			return nil, false
		}
		if st.IsDir() {
			result, err := w.Load(ctx, ingest.SourceDir(rootAbs, abs, false), ingest.MaterializeOptions{})
			if err != nil {
				return nil, false
			}
			return result, true
		}
		result, err := w.Load(ctx, ingest.SourceHop(rootAbs, abs), ingest.MaterializeOptions{})
		if err != nil {
			return nil, false
		}
		return result, true
	}

	scope, ok, err := ingest.NewResolver(rootAbs).ResolveScopeTarget(ctx, ref)
	if err != nil || !ok {
		return nil, false
	}
	result, err := w.Load(ctx, ingest.SourceDir(scope.Dir, "", false), ingest.MaterializeOptions{})
	if err != nil {
		return nil, false
	}
	return result, true
}
