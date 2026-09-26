package python

import (
	"context"
	"github.com/lewtec/patlint/pkg/project"
	"path"
	"path/filepath"
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
	pythonref "github.com/lewtec/patlint/pkg/reference/python"
)

const FamilyID = "python"

// Family is the Python family handle (file-as-module + package grain).
var Family = ingest.RegisterFamily(FamilyID, ingest.FamilySpec{
	Lattice:       pythonLattice{},
	ResolveImport: resolvePythonImport,
})

type pythonLattice struct{}

func (pythonLattice) Grains() []ingest.MoveGrain {
	return []ingest.MoveGrain{ingest.MoveGrainAtom, ingest.MoveGrainModule, ingest.MoveGrainPackage}
}

func (pythonLattice) ModuleKey(filePath string) string { return ingest.FileModuleKey(filePath) }

func (m pythonLattice) SameModule(a, b string) bool { return m.ModuleKey(a) == m.ModuleKey(b) }

func (pythonLattice) ListNodes(result *project.Result, grain ingest.MoveGrain, projectFamily string) []ingest.MoveNode {
	switch grain {
	case ingest.MoveGrainAtom:
		return ingest.ListAtomMoveNodes(result, projectFamily, func(name string) bool {
			leaf := ingest.AtomName(name)
			return len(leaf) >= 5 && strings.HasPrefix(leaf, "__") && strings.HasSuffix(leaf, "__")
		})
	case ingest.MoveGrainModule:
		return ingest.ListModuleFileMoveNodes(result, projectFamily)
	case ingest.MoveGrainPackage:
		return ingest.ListPackageMoveNodes(result, projectFamily, nil)
	default:
		return nil
	}
}

func init() {
	ingest.RegisterReferenceProvider("python", referenceProvider{})
}

func resolvePythonImport(sourcePath string, ctx ingest.ImportResolveContext) string {
	if ref, ok := (referenceProvider{}).Resolve(sourcePath, ctx); ok {
		return ref
	}
	return "python:" + sourcePath
}

type referenceProvider struct{}

func (referenceProvider) Name() string { return "python" }

func (referenceProvider) Resolve(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	if ref, ok := resolvePythonImportSpec(spec, ctx); ok {
		return ref, true
	}
	return "python:" + spec, true
}

func (referenceProvider) ResolveScopeTarget(ctx context.Context, ref ingest.Reference, rootDir string) (ingest.ProviderScopeTarget, bool, error) {
	if ref.Path == "" {
		return ingest.ProviderScopeTarget{}, false, nil
	}
	target, err := pythonref.ResolveModuleTarget(ctx, ref.Path, rootDir)
	if err != nil {
		return ingest.ProviderScopeTarget{}, true, err
	}
	canDescend := target.IsPackage
	// File pins single-module targets (os.py); without it callers Dir-walk Dir and
	// pull the whole stdlib when Dir is lib/python3.x.
	return ingest.ProviderScopeTarget{
		Dir:        target.Dir,
		File:       target.File,
		CanDescend: &canDescend,
	}, true, nil
}

func (referenceProvider) ResolveSymbolTarget(ctx context.Context, ref ingest.Reference, rootDir string) (ingest.ProviderSymbolTarget, bool, error) {
	target, ok, err := pythonref.ResolveSymbolTarget(ctx, ref.Path, ref.Name, rootDir)
	if !ok || err != nil {
		return ingest.ProviderSymbolTarget{}, ok, err
	}
	return ingest.ProviderSymbolTarget{Dir: target.Dir, Name: target.Name}, true, nil
}

func (referenceProvider) ListIngestRecursive(_ ingest.Reference, opts ingest.ListOptions) bool {
	return opts.Recursive
}

func (referenceProvider) AllowListEntity(ctx context.Context, ref ingest.Reference, _ ingest.Reference, entPath, language string, _ ingest.ListOptions) bool {
	if language != "python" {
		return false
	}
	// Listing/doc filter without workDir still works for stdlib; callers with
	// project context go through Resolver which passes rootDir at scope resolve.
	target, err := pythonref.ResolveModuleTarget(ctx, ref.Path, "")
	if err != nil {
		return false
	}
	return pythonref.MatchesEntityPath(target, entPath)
}

func (referenceProvider) ListOutputReference(ref ingest.Reference, entRef ingest.Reference) ingest.Reference {
	return ingest.Reference{Provider: ref.Provider, Path: ref.Path, Name: entRef.Name}
}

func (referenceProvider) DocIngestRecursive(ingest.Reference) bool { return false }

func (referenceProvider) AllowDocEntity(ctx context.Context, ref ingest.Reference, _ ingest.Reference, entPath, language string, _ []project.FamilyClaim) bool {
	if language != "python" {
		return false
	}
	target, err := pythonref.ResolveModuleTarget(ctx, ref.Path, "")
	if err != nil {
		return false
	}
	return pythonref.MatchesEntityPath(target, entPath)
}

func resolvePythonImportSpec(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	if strings.HasPrefix(spec, ".") {
		if ref, ok := resolvePythonRelativeImport(spec, ctx); ok {
			return ref, true
		}
		return "", false
	}
	if ref, ok := resolvePythonAbsoluteImport(spec, ctx); ok {
		return ref, true
	}
	// External / stdlib (unittest, json, …): keep a symbolic python: module ref.
	// Do NOT call ResolveModuleTarget here — that shells out to python3, and on
	// many hosts PATH python3 is a mise/uv shim that downloads tools and freezes
	// SeedResult / code view. importlib resolution stays for explicit python:
	// provider navigation (ResolveModuleTarget / AllowListEntity).
	if strings.TrimSpace(spec) == "" {
		return "", false
	}
	return "python:" + spec, true
}

func resolvePythonAbsoluteImport(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}

	candidates := []string{strings.ReplaceAll(spec, ".", "/")}
	if !strings.Contains(spec, "/") {
		candidates = append(candidates, spec)
	}

	for _, modulePath := range candidates {
		if modulePath == "" {
			continue
		}
		if ctx.KnownFiles[modulePath+".py"] {
			return ingest.FileRef("./" + modulePath + ".py"), true
		}
		if child, ok := ingest.KnownDirRepresentant(ctx.Representant, modulePath, ctx.KnownFiles); ok {
			return ingest.FileRef("./" + child), true
		}
	}
	return "", false
}

func resolvePythonRelativeImport(spec string, ctx ingest.ImportResolveContext) (string, bool) {
	level := 0
	for level < len(spec) && spec[level] == '.' {
		level++
	}
	if level == 0 {
		return "", false
	}

	tail := strings.TrimPrefix(spec, strings.Repeat(".", level))
	tail = strings.ReplaceAll(tail, ".", "/")

	baseDir := filepath.ToSlash(filepath.Dir(ctx.ImporterPath))
	if baseDir == "." {
		baseDir = ""
	}
	for i := 1; i < level; i++ {
		baseDir = path.Dir(baseDir)
		if baseDir == "." {
			baseDir = ""
		}
	}

	modulePath := baseDir
	if tail != "" {
		if modulePath == "" {
			modulePath = tail
		} else {
			modulePath = path.Join(modulePath, tail)
		}
	}
	if modulePath == "" {
		return "", false
	}

	if ctx.KnownFiles[modulePath+".py"] {
		return ingest.FileRef("./" + modulePath + ".py"), true
	}
	if child, ok := ingest.KnownDirRepresentant(ctx.Representant, modulePath, ctx.KnownFiles); ok {
		return ingest.FileRef("./" + child), true
	}
	return "", false
}
