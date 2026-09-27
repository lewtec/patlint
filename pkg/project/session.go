package project

import (
	"io/fs"
	"os"
	"path/filepath"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/sitter"
)

// Session is the project view: Root + fs.FS + parse Engine.
// NewSession(root) only; attach the engine with WithEngine at the entry.
type Session struct {
	Root string
	FS   fs.FS
	eng  sitter.Engine
}

// NewSession returns a view of root on the OS tree. Empty root means ".".
func NewSession(root string) *Session {
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil || abs == "" {
		abs = root
	}
	opened, err := lewpath.Open(abs)
	if err != nil {
		return &Session{Root: abs, FS: os.DirFS(abs)}
	}
	return &Session{Root: abs, FS: opened}
}

func (s *Session) clone() *Session {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}

// WithEngine returns a new Session with e. Nil e is allowed; parse rejects it.
func (s *Session) WithEngine(e sitter.Engine) *Session {
	out := s.clone()
	if out == nil {
		return nil
	}
	out.eng = e
	return out
}

// WithFS returns a new Session with fsys, keeping Root and Engine.
func (s *Session) WithFS(fsys fs.FS) *Session {
	out := s.clone()
	if out == nil {
		return nil
	}
	out.FS = fsys
	return out
}

// Engine is the parse backend set by WithEngine, or nil.
func (s *Session) Engine() sitter.Engine {
	if s == nil {
		return nil
	}
	return s.eng
}
