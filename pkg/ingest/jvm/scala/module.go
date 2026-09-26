package scala

import (
	"context"
	"os"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
	refpkg "github.com/lewtec/patlint/pkg/reference"
	scalaref "github.com/lewtec/patlint/pkg/reference/scala"
)

func init() {
	ingest.RegisterReferenceProvider("scala", referenceProvider{})
}

type referenceProvider struct{}

func (referenceProvider) Name() string { return "scala" }

func (referenceProvider) Resolve(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}
	ref := scalaref.ResolveImport(spec, ctx.KnownFiles)
	if ref == "" {
		return "", false
	}
	return ref, true
}

func (referenceProvider) ResolveScopeTarget(ctx context.Context, ref ingest.Reference, rootDir string) (ingest.ProviderScopeTarget, bool, error) {
	if ref.Path == "" {
		return ingest.ProviderScopeTarget{}, false, nil
	}
	target, err := scalaref.ResolveModuleTarget(ref.Path, rootDir)
	if err != nil {
		return ingest.ProviderScopeTarget{}, true, err
	}
	canDescend := target.File == ""
	return ingest.ProviderScopeTarget{Dir: target.Dir, CanDescend: &canDescend}, true, nil
}

func (referenceProvider) ResolveSymbolTarget(ctx context.Context, ref ingest.Reference, rootDir string) (ingest.ProviderSymbolTarget, bool, error) {
	target, ok, err := scalaref.ResolveSymbolTarget(ctx, ref.Path, ref.Name, rootDir)
	if !ok || err != nil {
		return ingest.ProviderSymbolTarget{}, ok, err
	}
	return ingest.ProviderSymbolTarget{Dir: target.Dir, Name: target.Name}, true, nil
}

func (referenceProvider) ListScopeChildren(ctx context.Context, ref ingest.Reference, rootDir string, includeHidden bool) ([]refpkg.ScopeChild, bool, error) {
	if ref.Path == "" {
		return nil, false, nil
	}
	target, err := scalaref.ResolveModuleTarget(ref.Path, rootDir)
	if err != nil {
		return nil, true, err
	}
	if target.File != "" {
		return nil, true, nil
	}

	entries, err := os.ReadDir(target.Dir)
	if err != nil {
		return nil, true, err
	}

	children := make([]refpkg.ScopeChild, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, true, err
		}
		name := entry.Name()
		if !includeHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			sub := lewpath.New(target.Dir, name).String()
			has, err := dirHasScalaSourcesRecursive(ctx, sub)
			if err != nil {
				return nil, true, err
			}
			if !has {
				continue
			}
			children = append(children, refpkg.ScopeChild{
				Ref:  ingest.Reference{Provider: "scala", Path: joinProviderPath(ref.Path, name)},
				Kind: refpkg.ScopeChildDir,
			})
			continue
		}
		if !isScalaFilename(name) {
			continue
		}
		typeName := strings.TrimSuffix(name, ".scala")
		children = append(children, refpkg.ScopeChild{
			Ref:  ingest.Reference{Provider: "scala", Path: joinProviderPath(ref.Path, typeName)},
			Kind: refpkg.ScopeChildFile,
		})
	}
	refpkg.SortScopeChildrenByPath(children)
	return children, true, nil
}

func (referenceProvider) ListIngestRecursive(_ ingest.Reference, opts ingest.ListOptions) bool {
	return opts.Recursive
}

func (referenceProvider) AllowListEntity(_ context.Context, _ ingest.Reference, _ ingest.Reference, entPath, language string, _ ingest.ListOptions) bool {
	return isScalaSourceEntity(entPath, language)
}

func (referenceProvider) ListOutputReference(ref ingest.Reference, entRef ingest.Reference) ingest.Reference {
	return ingest.Reference{Provider: ref.Provider, Path: ref.Path, Name: entRef.Name}
}

func (referenceProvider) DocIngestRecursive(ingest.Reference) bool { return false }

func (referenceProvider) AllowDocEntity(_ context.Context, _ ingest.Reference, _ ingest.Reference, entPath, language string, _ []project.FamilyClaim) bool {
	return isScalaSourceEntity(entPath, language)
}

func isScalaSourceEntity(entPath, language string) bool {
	return language == "scala" && strings.HasSuffix(entPath, ".scala")
}

func joinProviderPath(base, name string) string {
	return strings.ReplaceAll(refpkg.JoinProviderPath(base, name), "/", ".")
}

func dirHasScalaSourcesRecursive(ctx context.Context, dir string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, nil
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if entry.IsDir() {
			ok, err := dirHasScalaSourcesRecursive(ctx, lewpath.New(dir, entry.Name()).String())
			if err != nil || ok {
				return ok, err
			}
			continue
		}
		if isScalaFilename(entry.Name()) {
			return true, nil
		}
	}
	return false, nil
}

func isScalaFilename(name string) bool {
	return strings.HasSuffix(name, ".scala")
}
