package ignore

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/git-pkgs/gitignore"
)

// Collect walks root and builds an Engine.
//
// Evaluation order (last match wins):
//  1. built-in directory patterns (node_modules/, vendor/, …)
//  2. .git/info/exclude when present
//  3. .gitignore files under root (shallower paths first, then file order)
//  4. linguist-generated / refactree-ignored from every .gitattributes under root
//
// Each rule is appended to Rules and fed to gitignore.Matcher once (no second
// compile pass). Global core.excludesfile is not loaded.
func Collect(ctx context.Context, root string) (*Engine, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		abs = filepath.Dir(abs)
	}

	e := &Engine{Root: abs, m: gitignore.New("")}

	for _, name := range DefaultSkippedDirNames() {
		e.add(Rule{
			Pattern: name,
			DirOnly: true,
			BaseDir: abs,
			Source:  "builtin",
			Kind:    KindBuiltinDir,
		})
	}

	if exclude := lewpath.New(abs, ".git", "info", "exclude").String(); fileExists(exclude) {
		e.addGitignore(exclude, abs)
	}

	giFiles, err := findNamedFiles(ctx, abs, ".gitignore")
	if err != nil {
		return nil, err
	}
	sort.Slice(giFiles, func(i, j int) bool {
		di := strings.Count(giFiles[i], string(filepath.Separator))
		dj := strings.Count(giFiles[j], string(filepath.Separator))
		if di != dj {
			return di < dj
		}
		return giFiles[i] < giFiles[j]
	})
	for _, path := range giFiles {
		e.addGitignore(path, filepath.Dir(path))
	}

	attrFiles, err := findNamedFiles(ctx, abs, ".gitattributes")
	if err != nil {
		return nil, err
	}
	sort.Strings(attrFiles)
	for _, path := range attrFiles {
		attrs, err := parseGitAttributesFile(path)
		if err != nil {
			continue
		}
		e.Sources = append(e.Sources, path)
		base := filepath.Dir(path)
		for _, a := range attrs {
			pat := filepath.ToSlash(a.Pattern)
			dirOnly := strings.HasSuffix(pat, "/")
			e.add(Rule{
				Pattern: strings.TrimSuffix(pat, "/"),
				Negate:  !a.Ignore,
				DirOnly: dirOnly,
				BaseDir: base,
				Source:  path,
				Line:    a.Line,
				Kind:    a.Kind,
			})
		}
	}
	return e, nil
}

// New is an alias for Collect.
func New(ctx context.Context, root string) (*Engine, error) { return Collect(ctx, root) }

// add records r and compiles it into the matcher.
func (e *Engine) add(r Rule) {
	e.Rules = append(e.Rules, r)
	dir := ""
	if r.BaseDir != "" && r.BaseDir != e.Root {
		if rel, err := filepath.Rel(e.Root, r.BaseDir); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			dir = filepath.ToSlash(rel)
		}
	}
	e.m.AddPatterns([]byte(r.Display()+"\n"), dir)
}

// addGitignore parses path into Rules and compiles each line into the matcher.
func (e *Engine) addGitignore(path, baseDir string) {
	rs, err := parseGitignoreFile(path, baseDir)
	if err != nil || len(rs) == 0 {
		return
	}
	e.Sources = append(e.Sources, path)
	for _, r := range rs {
		e.add(r)
	}
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func findNamedFiles(ctx context.Context, root, name string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && IsSkippedDirName(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == name {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
