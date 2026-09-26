// Package projectfs is a small path-oriented file reader for project load and LSP overlays.
package projectfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FS reads project files by absolute (or cleaned) path.
type FS interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (fs.FileInfo, error)
	ReadDir(path string) ([]fs.DirEntry, error)
}

// OS is the real filesystem.
type OS struct{}

func (OS) ReadFile(path string) ([]byte, error)  { return os.ReadFile(path) }
func (OS) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }
func (OS) ReadDir(path string) ([]fs.DirEntry, error) {
	return os.ReadDir(path)
}

// Overlay prefers in-memory text for ReadFile; Stat reports overlay size when set.
// Walk/ReadDir still use the base tree (overlays do not create phantom files).
type Overlay struct {
	Base FS

	mu    sync.RWMutex
	files map[string][]byte // cleaned absolute path -> content
}

// NewOverlay wraps base (nil means OS).
func NewOverlay(base FS) *Overlay {
	if base == nil {
		base = OS{}
	}
	return &Overlay{Base: base, files: map[string][]byte{}}
}

func clean(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

// Set stores buffer bytes for path.
func (o *Overlay) Set(path string, content []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	cp := make([]byte, len(content))
	copy(cp, content)
	o.files[clean(path)] = cp
}

// SetString stores buffer text for path.
func (o *Overlay) SetString(path, content string) {
	o.Set(path, []byte(content))
}

// Delete removes an overlay entry.
func (o *Overlay) Delete(path string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.files, clean(path))
}

// Get returns overlay bytes if present.
func (o *Overlay) Get(path string) ([]byte, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	b, ok := o.files[clean(path)]
	if !ok {
		return nil, false
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp, true
}

// GetString returns overlay text if present.
func (o *Overlay) GetString(path string) (string, bool) {
	b, ok := o.Get(path)
	if !ok {
		return "", false
	}
	return string(b), true
}

// Paths returns cleaned absolute overlay paths.
func (o *Overlay) Paths() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]string, 0, len(o.files))
	for p := range o.files {
		out = append(out, p)
	}
	return out
}

// Clone returns a shallow copy of overlay entries on the same base.
func (o *Overlay) Clone() *Overlay {
	o.mu.RLock()
	defer o.mu.RUnlock()
	n := NewOverlay(o.Base)
	for p, b := range o.files {
		cp := make([]byte, len(b))
		copy(cp, b)
		n.files[p] = cp
	}
	return n
}

func (o *Overlay) ReadFile(path string) ([]byte, error) {
	p := clean(path)
	o.mu.RLock()
	if b, ok := o.files[p]; ok {
		cp := make([]byte, len(b))
		copy(cp, b)
		o.mu.RUnlock()
		return cp, nil
	}
	o.mu.RUnlock()
	return o.Base.ReadFile(p)
}

func (o *Overlay) Stat(path string) (fs.FileInfo, error) {
	p := clean(path)
	o.mu.RLock()
	if b, ok := o.files[p]; ok {
		o.mu.RUnlock()
		return overlayInfo{name: filepath.Base(p), size: int64(len(b)), mod: time.Now()}, nil
	}
	o.mu.RUnlock()
	return o.Base.Stat(p)
}

func (o *Overlay) ReadDir(path string) ([]fs.DirEntry, error) {
	return o.Base.ReadDir(clean(path))
}

type overlayInfo struct {
	name string
	size int64
	mod  time.Time
}

func (i overlayInfo) Name() string       { return i.name }
func (i overlayInfo) Size() int64        { return i.size }
func (i overlayInfo) Mode() fs.FileMode  { return 0o644 }
func (i overlayInfo) ModTime() time.Time { return i.mod }
func (i overlayInfo) IsDir() bool        { return false }
func (i overlayInfo) Sys() any           { return nil }
