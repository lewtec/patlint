package ingest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestParseFile_TreeSitterFaultIsError(t *testing.T) {
	// Known pure-Go grammar fault on this file (slice/variadic near out[1:]...).
	// Packs (language_go.rft) attribute **/*.go → go; no fake PackQueries.
	root, err := filepath.Abs("../..")
	require.NoError(t, err)

	wd, _ := os.Getwd()
	if strings.HasSuffix(filepath.ToSlash(wd), "/pkg/ingest") {
		root, _ = filepath.Abs(lewpath.New(wd, "../..").String())
	}
	abs := lewpath.New(root, "internal/fuzzy/catalog.go").String()
	if _, err := os.Stat(abs); err != nil {
		t.Skip(err)
	}

	// Should not SIGSEGV the process. Real pack policy only.
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(root).WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	err = w.WalkExtracts(t.Context(), ingest.SourceHop(root, abs), func(*project.FileExtract) bool {
		return true
	})
	if err == nil {
		// Grammar may be fixed upstream; success is fine.
		t.Log("parse succeeded; no fault on this grammar version")
		return
	}
	require.ErrorIs(t, err, ingest.ErrTreeSitterFault,
		"want tree-sitter fault error, got %v", err)

}
