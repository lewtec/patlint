// Package walker is the Session+LispVM read/nav spine: NewWalker(sess, vm).
package walker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
)

// Doc is signature + docstring for a reference (SPEC: hop/dir, not full project).
func (w *Walker) Doc(ctx context.Context, dir, reference string) (*ingest.DocResult, error) {
	if w == nil || w.Sess == nil {
		return nil, ingest.ErrNilSession
	}
	if w.VM == nil {
		return nil, fmt.Errorf("%w: walker: nil LispVM", pattern.ErrExtract)
	}
	rawRef := ingest.ParseReference(reference)
	if rawRef.Provider != "" && rawRef.Provider != "path" {
		if rawRef.Name != "" {
			target, ok, err := ingest.NewResolver("").ResolveSymbolTarget(ctx, rawRef)
			if err != nil {
				return nil, err
			}
			if ok {
				return w.docForProviderSymbol(ctx, rawRef, target)
			}
		}
		return nil, fmt.Errorf("%w: %s", ingest.ErrProviderDocNeedsSymbol, reference)
	}

	rawRef = ingest.NormalizePathReference(rawRef)
	absPath := rawRef.Path
	if !filepath.IsAbs(absPath) {
		absPath = lewpath.New(dir, strings.TrimPrefix(rawRef.Path, "./")).String()
	}

	st, err := os.Stat(absPath)
	if err != nil {
		canon := w.CanonicalizeReference(ctx, dir, rawRef)
		cAbs := canon.Path
		if !filepath.IsAbs(cAbs) {
			cAbs = lewpath.New(dir, strings.TrimPrefix(canon.Path, "./")).String()
		}
		result, err := w.Load(ctx, ingest.SourceSeed(dir, cAbs), ingest.MaterializeOptions{})
		if err != nil {
			return nil, err
		}
		return ingest.DocFromResult(ctx, w.VM, dir, result, canon)
	}

	if st.IsDir() {
		result, err := w.Load(ctx, ingest.SourceDir(dir, absPath, true), ingest.MaterializeOptions{})
		if err != nil {
			return nil, err
		}
		ref, err := ingest.CanonicalSourceReference(dir, result, rawRef, w.VM)
		if err != nil {
			return nil, err
		}
		return ingest.DocFromResult(ctx, w.VM, dir, result, ref)
	}

	canon := w.CanonicalizeReference(ctx, dir, rawRef)
	cAbs := canon.Path
	if !filepath.IsAbs(cAbs) {
		cAbs = lewpath.New(dir, strings.TrimPrefix(canon.Path, "./")).String()
	}
	result, err := w.Load(ctx, ingest.SourceSeed(dir, cAbs), ingest.MaterializeOptions{})
	if err != nil {
		return nil, err
	}
	return ingest.DocFromResult(ctx, w.VM, dir, result, canon)
}

func (w *Walker) docForProviderSymbol(ctx context.Context, ref ingest.Reference, target ingest.ProviderSymbolTarget) (*ingest.DocResult, error) {
	result, err := w.Load(ctx, ingest.SourceDir(target.Dir, "", ingest.ProviderDocIngestRecursive(ref)), ingest.MaterializeOptions{
		ExpandImports: true,
	})
	if err != nil {
		return nil, err
	}
	return ingest.DocFromProviderResult(ctx, w.VM, result, ref, target)
}
