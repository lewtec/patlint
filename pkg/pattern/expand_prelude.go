package pattern

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	lewpath "github.com/lewtec/lewkit/x/path"
)

// expandPrelude holds expand-time defs from loaded .rft (core.rft via LoadFiles).
// Expand without a pack load reads that same official core.rft from disk.
var (
	expandPreludeMu  sync.RWMutex
	expandPrelude    expandEnv
	officialCoreOnce sync.Once
)

// SetExpandPrelude replaces the expand-time prelude environment (defs only).
func SetExpandPrelude(env expandEnv) {
	expandPreludeMu.Lock()
	defer expandPreludeMu.Unlock()
	if env == nil {
		expandPrelude = expandEnv{}
		return
	}
	expandPrelude = env.clone()
}

func installExpandMacros(src string) error {
	forms, err := ParseSexpDataFile(src)
	if err != nil {
		return fmt.Errorf("pattern: expand macros: %w", err)
	}
	e := expandEnv{}
	for _, raw := range forms {
		form, ok := raw.([]any)
		if !ok || len(form) == 0 {
			continue
		}
		h, _ := form[0].(string)
		if h != "def" {
			continue
		}
		n, fn, err := parseDefForm(form[1:])
		if err != nil {
			return fmt.Errorf("pattern: expand macros: %w", err)
		}
		e[n] = fn
	}
	SetExpandPrelude(e)
	return nil
}

func installOfficialCoreMacros() {
	src, err := readOfficialCoreRft()
	if err != nil {
		panic("pattern: core.rft: " + err.Error())
	}
	if err := installExpandMacros(src); err != nil {
		panic(err)
	}
}

func readOfficialCoreRft() (string, error) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("no caller path")
	}
	p := lewpath.New(filepath.Dir(self), "..", "..", "internal", "prelude", "core.rft").String()
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// withExpandPrelude merges the installed expand prelude under env (caller wins).
func withExpandPrelude(env expandEnv) expandEnv {
	expandPreludeMu.RLock()
	base := expandPrelude
	expandPreludeMu.RUnlock()
	if len(base) == 0 {
		officialCoreOnce.Do(installOfficialCoreMacros)
		expandPreludeMu.RLock()
		base = expandPrelude
		expandPreludeMu.RUnlock()
	}
	out := base.clone()
	if out == nil {
		out = expandEnv{}
	}
	for k, v := range env {
		out[k] = v
	}
	return out
}
