package ingest

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/projectfs"
)

// sessionProjectFS adapts Session.FS (rooted io/fs.FS) to abs-path projectfs.FS.
func sessionProjectFS(s *project.Session) projectfs.FS {
	if s == nil || s.FS == nil {
		return projectfs.OS{}
	}
	return rootedFS{root: s.Root, fsys: s.FS}
}

type rootedFS struct {
	root string
	fsys fs.FS
}

func (r rootedFS) rel(path string) (string, bool) {
	rel, err := filepath.Rel(r.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func (r rootedFS) ReadFile(path string) ([]byte, error) {
	name, ok := r.rel(path)
	if !ok {
		return os.ReadFile(path)
	}
	return fs.ReadFile(r.fsys, name)
}

func (r rootedFS) Stat(path string) (fs.FileInfo, error) {
	name, ok := r.rel(path)
	if !ok {
		return os.Stat(path)
	}
	return fs.Stat(r.fsys, name)
}

func (r rootedFS) ReadDir(path string) ([]fs.DirEntry, error) {
	name, ok := r.rel(path)
	if !ok {
		return os.ReadDir(path)
	}
	if name == "." || name == "" {
		return fs.ReadDir(r.fsys, ".")
	}
	return fs.ReadDir(r.fsys, name)
}
