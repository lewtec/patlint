package pythonref

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	lewpath "github.com/lewtec/lewkit/x/path"
)

// Tabled errors for ResolveModuleTarget / pythonCommand and callers (errors.Is).
var (
	ErrPythonExecutableNotFound = errors.New("python executable not found")
	ErrNoFilesystemSource       = errors.New("has no filesystem source")
	ErrEmptyModulePath          = errors.New("python provider module path is empty")
	ErrModuleResolveTimeout     = errors.New("python module resolution timed out")
	ErrModuleNotFound           = errors.New("python module not found")
	ErrInvalidResolverOutput    = errors.New("invalid python resolver output")
	ErrEmptyPackageDirectory    = errors.New("python module resolved to empty package directory")
	ErrInvalidSource            = errors.New("python module resolved to invalid source")
)

type ModuleTarget struct {
	Dir       string
	File      string
	IsPackage bool
}

type SymbolTarget struct {
	Dir  string
	Name string
}

var moduleTargetCache sync.Map

// defaultProjectRoot is the serve/browse --dir (or last non-empty resolver root).
// Used when a call site passes workDir "" so we still resolve with the project venv,
// not the process cwd / system python.
var (
	defaultProjectRootMu sync.RWMutex
	defaultProjectRoot   string
)

// SetDefaultProjectRoot records the project tree used for python resolution when
// workDir is omitted (list/doc filters, etc.). Safe to call from NewResolver/serve.
func SetDefaultProjectRoot(root string) {
	root = strings.TrimSpace(root)
	if root == "" {
		return
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	defaultProjectRootMu.Lock()
	defaultProjectRoot = root
	defaultProjectRootMu.Unlock()
}

func effectiveWorkDir(workDir string) string {
	workDir = strings.TrimSpace(workDir)
	if workDir != "" {
		if abs, err := filepath.Abs(workDir); err == nil {
			return abs
		}
		return workDir
	}
	defaultProjectRootMu.RLock()
	root := defaultProjectRoot
	defaultProjectRootMu.RUnlock()
	return root
}

type moduleResolveResult struct {
	Origin     string   `json:"origin"`
	Locations  []string `json:"locations"`
	ModuleFile string   `json:"module_file"`
	SourceFile string   `json:"source_file"`
}

const moduleResolveScript = `
import importlib.util
import importlib
import inspect
import json
import sys

name = sys.argv[1]
spec = importlib.util.find_spec(name)
if spec is None:
    print(json.dumps({"error": "not_found", "name": name}))
    sys.exit(2)

origin = spec.origin
locations = list(spec.submodule_search_locations or [])
module_file = None
source_file = None
try:
    mod = importlib.import_module(name)
    module_file = getattr(mod, "__file__", None)
    try:
        source_file = inspect.getsourcefile(mod)
    except Exception as e:
        print(f"Failed to get source file: {e}", file=sys.stderr)
        source_file = None
except Exception as e:
    print(f"Failed to import module: {e}", file=sys.stderr)

print(json.dumps({
    "origin": origin,
    "locations": locations,
    "module_file": module_file,
    "source_file": source_file,
}))
`

// ResolveModuleTarget resolves a python:<module> spec into a concrete ingest
// scope directory plus the module file name used for symbol/doc filtering.
// workDir is the project root (serve/browse --dir); empty falls back to
// SetDefaultProjectRoot (not process cwd). Resolution prefers that tree's
// .venv/bin/python so site-packages match the project.
func ResolveModuleTarget(ctx context.Context, spec, workDir string) (ModuleTarget, error) {
	module := normalizeModuleSpec(spec)
	if module == "" {
		return ModuleTarget{}, ErrEmptyModulePath
	}

	workDir = effectiveWorkDir(workDir)

	cacheKey := workDir + "\x00" + module
	if cached, ok := moduleTargetCache.Load(cacheKey); ok {
		return cached.(ModuleTarget), nil
	}

	target, err := resolveModuleTarget(ctx, module, workDir)
	if err != nil {
		return ModuleTarget{}, err
	}

	moduleTargetCache.Store(cacheKey, target)
	return target, nil
}

// ResolveSymbolTarget resolves python:<module>::<symbol> to an ingest target.
func ResolveSymbolTarget(ctx context.Context, spec, symbol, workDir string) (SymbolTarget, bool, error) {
	if symbol == "" {
		return SymbolTarget{}, false, nil
	}
	target, err := ResolveModuleTarget(ctx, spec, workDir)
	if err != nil {
		return SymbolTarget{}, true, err
	}
	return SymbolTarget{Dir: target.Dir, Name: symbol}, true, nil
}

// MatchesEntityPath reports whether entPath (relative to target.Dir) belongs to
// the resolved module target.
func MatchesEntityPath(target ModuleTarget, entPath string) bool {
	rel := filepath.ToSlash(entPath)
	if target.File == "" {
		return true
	}
	return rel == filepath.ToSlash(target.File)
}

func resolveModuleTarget(ctx context.Context, module, workDir string) (ModuleTarget, error) {
	cmdPath, err := pythonCommand(workDir)
	if err != nil {
		return ModuleTarget{}, err
	}
	cmd := exec.CommandContext(ctx, cmdPath, "-c", moduleResolveScript, module)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = pythonCommandEnv(workDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return ModuleTarget{}, fmt.Errorf("%w: %s", ErrModuleNotFound, msg)
		}
		return ModuleTarget{}, fmt.Errorf("%w %q", ErrModuleNotFound, module)
	}

	var result moduleResolveResult
	if err := json.Unmarshal(out, &result); err != nil {
		return ModuleTarget{}, fmt.Errorf("%w: %w", ErrInvalidResolverOutput, err)
	}

	if len(result.Locations) > 0 {
		dir := result.Locations[0]
		if dir == "" {
			return ModuleTarget{}, fmt.Errorf("%w %q", ErrEmptyPackageDirectory, module)
		}
		file := ""
		origin := strings.TrimSpace(result.Origin)
		if origin != "" && origin != "built-in" && origin != "frozen" {
			originDir := filepath.Dir(origin)
			if samePath(originDir, dir) {
				file = filepath.Base(origin)
			}
		}
		if file == "" {
			for _, candidate := range []string{result.ModuleFile, result.SourceFile} {
				if candidate == "" {
					continue
				}
				candidateDir := filepath.Dir(candidate)
				if samePath(candidateDir, dir) {
					file = filepath.Base(candidate)
					break
				}
			}
		}
		if file == "" {
			initFile := lewpath.New(dir, "__init__.py").String()
			if st, err := os.Stat(initFile); err == nil && !st.IsDir() {
				file = "__init__.py"
			}
		}
		return ModuleTarget{Dir: dir, File: file, IsPackage: true}, nil
	}

	origin := strings.TrimSpace(result.Origin)
	if origin == "" || origin == "built-in" || origin == "frozen" {
		for _, candidate := range []string{result.ModuleFile, result.SourceFile} {
			if candidate == "" {
				continue
			}
			st, err := os.Stat(candidate)
			if err == nil && !st.IsDir() {
				return ModuleTarget{Dir: filepath.Dir(candidate), File: filepath.Base(candidate), IsPackage: false}, nil
			}
		}
		return ModuleTarget{}, fmt.Errorf("python module %q %w", module, ErrNoFilesystemSource)
	}
	st, err := os.Stat(origin)
	if err != nil || st.IsDir() {
		return ModuleTarget{}, fmt.Errorf("%w %q: %q", ErrInvalidSource, module, origin)
	}
	return ModuleTarget{Dir: filepath.Dir(origin), File: filepath.Base(origin), IsPackage: false}, nil
}

// pythonCommand returns an interpreter to run for module resolution.
// Prefers project virtualenv python under workDir (walk parents). PATH is last
// resort and skips mise/uv shims (e.g. `mise exec uv -- uv run python3`) that
// download tools and hang interactive serve when a project has mise.toml + uv.
// Does not consult process cwd when workDir is set — serve --dir is authoritative.
func pythonCommand(workDir string) (string, error) {
	workDir = effectiveWorkDir(workDir)
	if py, ok := findVenvPython(workDir); ok {
		return py, nil
	}
	for _, name := range []string{"python3", "python"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if isMiseOrUvPythonShim(path) {
			continue
		}
		return path, nil
	}
	return "", fmt.Errorf("%w (tried .venv/venv under project root; PATH python3/python skipped if mise/uv shims)", ErrPythonExecutableNotFound)
}

// isMiseOrUvPythonShim reports scripts that re-exec through mise and/or `uv run`.
// Those can block for minutes downloading tools when the project mise.toml pins uv.
func isMiseOrUvPythonShim(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Real CPython is a large binary; shims are short #! scripts.
	if len(data) > 4096 {
		return false
	}
	s := string(data)
	if !strings.HasPrefix(strings.TrimSpace(s), "#!") {
		return false
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "mise") {
		return true
	}
	if strings.Contains(lower, "uv run") || strings.Contains(lower, "uvx ") {
		return true
	}
	return false
}

// findVenvPython looks for .venv|venv/bin/python{,3} starting at projectRoot,
// walking parents (monorepo layouts). Empty projectRoot returns false (no cwd fallback).
func findVenvPython(projectRoot string) (string, bool) {
	if projectRoot == "" {
		return "", false
	}
	if abs, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = abs
	}
	for dir := projectRoot; ; {
		for _, rel := range []string{
			lewpath.New(".venv", "bin", "python").String(),
			lewpath.New(".venv", "bin", "python3").String(),
			lewpath.New("venv", "bin", "python").String(),
			lewpath.New("venv", "bin", "python3").String(),
		} {
			candidate := lewpath.New(dir, rel).String()
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// venvRootForPython returns the venv directory (…/.venv) given …/.venv/bin/python, or "".
func venvRootForPython(pyPath string) string {
	// …/bin/python → …/
	bin := filepath.Dir(pyPath)
	if filepath.Base(bin) != "bin" {
		return ""
	}
	return filepath.Dir(bin)
}

func pythonCommandEnv(workDir string) []string {
	env := os.Environ()
	base := lewpath.New(os.TempDir(), "refactree-python").String()
	uvCache := lewpath.New(base, "uv-cache").String()
	xdgCache := lewpath.New(base, "xdg-cache").String()
	// Best-effort cache dirs for uv/mise shims; failures leave env unset below.
	if err := os.MkdirAll(uvCache, 0o755); err == nil {
		env = append(env, "UV_CACHE_DIR="+uvCache)
	}
	if err := os.MkdirAll(xdgCache, 0o755); err == nil {
		env = append(env, "XDG_CACHE_HOME="+xdgCache)
	}

	proj := effectiveWorkDir(workDir)
	if proj != "" {
		pyPath := proj
		if existing := os.Getenv("PYTHONPATH"); existing != "" {
			pyPath = proj + string(os.PathListSeparator) + existing
		}
		env = append(env, "PYTHONPATH="+pyPath)
	}

	// VIRTUAL_ENV from the venv we run; site-packages come from that python binary.
	if py, ok := findVenvPython(proj); ok {
		if v := venvRootForPython(py); v != "" {
			env = append(env, "VIRTUAL_ENV="+v)
		}
	}
	return env
}

func normalizeModuleSpec(spec string) string {
	module := strings.TrimSpace(spec)
	module = strings.Trim(module, "/")
	module = strings.ReplaceAll(module, "/", ".")
	module = strings.Trim(module, ".")
	return module
}

func samePath(a, b string) bool {
	aa := filepath.Clean(a)
	bb := filepath.Clean(b)
	return aa == bb
}
