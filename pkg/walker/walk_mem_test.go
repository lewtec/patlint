package walker

import (
	"testing"
	"testing/fstest"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
	"github.com/stretchr/testify/require"
)

func TestWalkAtoms_MapFS(t *testing.T) {
	root := t.TempDir()
	memory := fstest.MapFS{
		"a.go":     {Data: []byte("package p\n\nfunc Listed() {}\n")},
		"note.txt": {Data: []byte("not code\n")},
		"sub/b.go": {Data: []byte("package sub\n\nfunc Nested() {}\n")},
	}
	session := project.NewSession(root).WithFS(memory).WithEngine(ccgo.Engine{})
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)
	fileWalker, err := NewWalker(t.Context(), session, vm)
	require.NoError(t, err)

	var references []string
	err = fileWalker.WalkAtoms(t.Context(), root, "", ingest.ListOptions{Recursive: true}, func(info ingest.AtomInfo) bool {
		references = append(references, info.Atom.Reference)
		return true
	})
	require.NoError(t, err)
	require.Contains(t, references, "path:./a.go::Listed")
	require.Contains(t, references, "path:./sub/b.go::Nested")
}
