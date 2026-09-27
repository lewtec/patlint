package project

import (
	"bytes"
	"io/fs"
	"path"
	"strings"
	"time"
)

// PatchFS is an immutable layer over parent (SPEC.md). Write + file Rename.
type PatchFS struct {
	parent  fs.FS
	writes  map[string][]byte
	renames map[string]string // from → to (files only)
}

// NewPatchFS copies writes (slash names, relative to Root) over parent.
func NewPatchFS(parent fs.FS, writes map[string][]byte) *PatchFS {
	return NewPatchFSLayer(parent, writes, nil)
}

// NewPatchFSLayer is Write + file Rename. Dest parent must exist at WriteBack.
func NewPatchFSLayer(parent fs.FS, writes map[string][]byte, renames map[string]string) *PatchFS {
	m := make(map[string][]byte, len(writes))
	for k, v := range writes {
		name := path.Clean(strings.TrimPrefix(k, "./"))
		if name == "." || name == "" {
			continue
		}
		cp := make([]byte, len(v))
		copy(cp, v)
		m[name] = cp
	}
	rn := make(map[string]string, len(renames))
	for from, to := range renames {
		f := path.Clean(strings.TrimPrefix(from, "./"))
		t := path.Clean(strings.TrimPrefix(to, "./"))
		if f == "." || f == "" || t == "." || t == "" || f == t {
			continue
		}
		rn[f] = t
	}
	return &PatchFS{parent: parent, writes: m, renames: rn}
}

func (p *PatchFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	name = path.Clean(name)
	if p == nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if b, ok := p.writes[name]; ok {
		return &memFile{name: path.Base(name), r: bytes.NewReader(b), size: int64(len(b))}, nil
	}
	if _, ok := p.renames[name]; ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if p.parent != nil {
		for from, to := range p.renames {
			if to == name {
				return p.parent.Open(from)
			}
		}
		return p.parent.Open(name)
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

type memFile struct {
	name string
	r    *bytes.Reader
	size int64
}

func (f *memFile) Stat() (fs.FileInfo, error) {
	return memInfo{name: f.name, size: f.size}, nil
}

func (f *memFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *memFile) Close() error               { return nil }

type memInfo struct {
	name string
	size int64
}

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) Mode() fs.FileMode  { return 0o644 }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return false }
func (i memInfo) Sys() any           { return nil }

// OverlayWrites is last Write per name in a PatchFS stack (child wins).
func OverlayWrites(fsys fs.FS) map[string][]byte {
	p, ok := fsys.(*PatchFS)
	if !ok || p == nil {
		return map[string][]byte{}
	}
	out := OverlayWrites(p.parent)
	for k, v := range p.writes {
		cp := make([]byte, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

// OverlayRenames is file Rename from→to in a PatchFS stack (child wins).
func OverlayRenames(fsys fs.FS) map[string]string {
	p, ok := fsys.(*PatchFS)
	if !ok || p == nil {
		return map[string]string{}
	}
	out := OverlayRenames(p.parent)
	for from, to := range p.renames {
		out[from] = to
	}
	return out
}
