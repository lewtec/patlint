package nix

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/lewtec/patlint/pkg/ingest"
	refpkg "github.com/lewtec/patlint/pkg/reference"
	nixref "github.com/lewtec/patlint/pkg/reference/nix"
)

const FamilyID = "nix"

// Family is the Nix family handle (file-as-module lattice).
var Family = ingest.RegisterFamily(FamilyID, ingest.FamilySpec{
	Lattice:       lattice{},
	ResolveImport: resolveNixImport,
})

type lattice struct{}

func (lattice) Grains() []ingest.MoveGrain {
	return []ingest.MoveGrain{ingest.MoveGrainAtom, ingest.MoveGrainModule}
}

func (lattice) ModuleKey(filePath string) string { return ingest.FileModuleKey(filePath) }

func (m lattice) SameModule(a, b string) bool { return m.ModuleKey(a) == m.ModuleKey(b) }

func (lattice) ListNodes(result *project.Result, grain ingest.MoveGrain, projectFamily string) []ingest.MoveNode {
	switch grain {
	case ingest.MoveGrainAtom:
		// Skip dotted attrpaths (foo.bar) and inherit-only noise; v1 renames
		// simple top-level identifiers only.
		return ingest.ListAtomMoveNodes(result, projectFamily, func(name string) bool {
			return strings.Contains(name, ".")
		})
	case ingest.MoveGrainModule:
		return ingest.ListModuleFileMoveNodes(result, projectFamily)
	default:
		return nil
	}
}

func init() {
	ingest.RegisterReferenceProvider("nix", referenceProvider{})
}

func resolveNixImport(sourcePath string, ctx ingest.ImportResolveContext) string {
	if ref, ok := resolveNixPathImport(sourcePath, ctx); ok {
		return ref
	}
	if ref, ok := (referenceProvider{}).Resolve(sourcePath, ctx); ok {
		return ref
	}
	return "nix:" + sourcePath
}

type referenceProvider struct{}

func (referenceProvider) Name() string { return "nix" }

func (referenceProvider) Resolve(spec string, _ ingest.ImportResolveContext) (string, bool) {
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") || strings.HasPrefix(spec, "/") {
		return "", false
	}
	spec = normalizeProviderSpec(spec)
	if spec == "" {
		return "", false
	}
	return "nix:" + spec, true
}

func (referenceProvider) ResolveScopeTarget(ctx context.Context, ref ingest.Reference, _ string) (ingest.ProviderScopeTarget, bool, error) {
	if ref.Path == "" {
		return ingest.ProviderScopeTarget{}, false, nil
	}
	target, err := nixref.ResolveTarget(ref.Path)
	if err != nil {
		return ingest.ProviderScopeTarget{}, true, err
	}
	canDescend := target.IsDir && (target.File == "" || nixProviderRootScope(ref.Path))
	return ingest.ProviderScopeTarget{Dir: target.Dir, CanDescend: &canDescend}, true, nil
}

func (referenceProvider) ResolveSymbolTarget(ctx context.Context, ref ingest.Reference, _ string) (ingest.ProviderSymbolTarget, bool, error) {
	target, ok, err := nixref.ResolveSymbolTarget(ctx, ref.Path, ref.Name)
	if !ok || err != nil {
		return ingest.ProviderSymbolTarget{}, ok, err
	}
	return ingest.ProviderSymbolTarget{Dir: target.Dir, Name: target.Name}, true, nil
}

func (referenceProvider) ListScopeChildren(ctx context.Context, ref ingest.Reference, _ string, includeHidden bool) ([]refpkg.ScopeChild, bool, error) {
	if ref.Path == "" {
		return nil, false, nil
	}
	target, err := nixref.ResolveTarget(ref.Path)
	if err != nil {
		return nil, true, err
	}
	if !target.IsDir {
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

		childRef := ingest.Reference{Provider: "nix", Path: refpkg.JoinProviderPath(ref.Path, name)}
		if entry.IsDir() {
			children = append(children, refpkg.ScopeChild{
				Ref:  childRef,
				Kind: refpkg.ScopeChildDir,
			})
			continue
		}
		if !isNixSourceFile(name) {
			continue
		}
		children = append(children, refpkg.ScopeChild{
			Ref:  childRef,
			Kind: refpkg.ScopeChildFile,
		})
	}

	refpkg.SortScopeChildrenByKindThenPath(children)
	return children, true, nil
}

func (referenceProvider) ListIngestRecursive(_ ingest.Reference, opts ingest.ListOptions) bool {
	return opts.Recursive
}

func (referenceProvider) AllowListEntity(_ context.Context, ref ingest.Reference, _ ingest.Reference, entPath, language string, _ ingest.ListOptions) bool {
	if language != "nix" {
		return false
	}
	target, err := nixref.ResolveTarget(ref.Path)
	if err != nil {
		return false
	}
	return nixref.MatchesEntityPath(target, entPath)
}

func (referenceProvider) ListOutputReference(ref ingest.Reference, entRef ingest.Reference) ingest.Reference {
	return ingest.Reference{Provider: ref.Provider, Path: ref.Path, Name: entRef.Name}
}

func (referenceProvider) DocIngestRecursive(ingest.Reference) bool { return false }

func (referenceProvider) AllowDocEntity(_ context.Context, ref ingest.Reference, _ ingest.Reference, entPath, language string, _ []project.FamilyClaim) bool {
	if language != "nix" {
		return false
	}
	target, err := nixref.ResolveTarget(ref.Path)
	if err != nil {
		return false
	}
	return nixref.MatchesEntityPath(target, entPath)
}

// resolveNixPathImport maps a Nix import path argument to a product path: ref.
//
//	./. or .     → directory of the importer; if that dir has default.nix, that file
//	./foo, ../x  → relative to importer dir; directories with default.nix expand to it
//	/abs/…       → absolute filesystem path (PathReferenceForAbsolute when under root)
//
// Matches Nix: importing a directory loads default.nix inside it when present.
// Angle paths (<nixpkgs/lib>) and string interpolations are not handled here.
func resolveNixPathImport(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}

	// Absolute filesystem path (not project-relative).
	if strings.HasPrefix(spec, "/") {
		rootAbs, err := filepath.Abs(ctx.RootDir)
		if err != nil {
			return ingest.FileRef(filepath.ToSlash(spec)), true
		}
		if resolved, ok := resolveNixPathOnDisk(spec, "", ctx); ok {
			return ingest.PathReferenceForAbsolute(rootAbs, resolved), true
		}
		return ingest.FileRef(filepath.ToSlash(spec)), true
	}

	// Relative to the importing file's directory.
	if !(strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") || spec == "." || spec == "./.") {
		return "", false
	}

	// RelativeImportPath joins spec against Dir(importer):
	//   pkg/a.nix + ./. → pkg
	//   pkg/a.nix + ./b → pkg/b  (dir or file; default.nix expanded below)
	rel := ingest.RelativeImportPath(ctx.ImporterPath, spec)
	if rel == "" || rel == "." {
		// Project root as path: if default.nix is known / on disk, use it.
		if ref, ok := resolveKnownNixPath(".", ctx); ok {
			return ref, true
		}
		rootAbs, err := filepath.Abs(ctx.RootDir)
		if err == nil {
			if resolved, ok := resolveNixPathOnDisk(rootAbs, "", ctx); ok {
				return ingest.PathReferenceForAbsolute(rootAbs, resolved), true
			}
		}
		return ingest.FileRef("./"), true
	}

	if ref, ok := resolveKnownNixPath(rel, ctx); ok {
		return ref, true
	}

	rootAbs, err := filepath.Abs(ctx.RootDir)
	if err != nil {
		return ingest.FileRef("./" + rel), true
	}
	candidate := lewpath.New(rootAbs, filepath.FromSlash(rel)).String()
	if resolved, ok := resolveNixPathOnDisk(candidate, rel, ctx); ok {
		return ingest.PathReferenceForAbsolute(rootAbs, resolved), true
	}

	return ingest.FileRef("./" + rel), true
}

func resolveKnownNixPath(rel string, ctx ingest.ImportResolveContext) (string, bool) {
	knownFiles := ctx.KnownFiles
	if knownFiles == nil {
		return "", false
	}
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	if rel == "" || rel == "." {
		if child, ok := ingest.KnownDirRepresentant(ctx.Representant, "", knownFiles); ok {
			return ingest.FileRef("./" + child), true
		}
		return "", false
	}
	if knownFiles[rel] {
		return ingest.FileRef("./" + rel), true
	}
	if child, ok := ingest.KnownDirRepresentant(ctx.Representant, rel, knownFiles); ok {
		return ingest.FileRef("./" + child), true
	}
	return "", false
}

// resolveNixPathOnDisk resolves a path on disk. Directory imports expand to
// the pack as-directory-representant file when present.
func resolveNixPathOnDisk(baseAbs, dirRel string, ctx ingest.ImportResolveContext) (string, bool) {
	st, err := osStat(baseAbs)
	if err != nil {
		if st2, err2 := osStat(baseAbs + ".nix"); err2 == nil && !st2.IsDir() {
			return baseAbs + ".nix", true
		}
		return "", false
	}
	if st.IsDir() {
		if base, ok := ingest.DiskDirRepresentant(ctx.Representant, baseAbs, dirRel); ok {
			return lewpath.New(baseAbs, filepath.FromSlash(base)).String(), true
		}
		return baseAbs, true
	}
	return baseAbs, true
}

var osStat = os.Stat

func normalizeProviderSpec(spec string) string {
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "<") && strings.HasSuffix(spec, ">") {
		spec = strings.TrimPrefix(strings.TrimSuffix(spec, ">"), "<")
	}
	return strings.Trim(spec, "/")
}

func nixProviderRootScope(spec string) bool {
	spec = normalizeProviderSpec(spec)
	return spec != "" && !strings.Contains(spec, "/")
}

func isNixSourceFile(name string) bool {
	return strings.HasSuffix(name, ".nix")
}
