package ingest_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestParseFile_TreeSitterFaultIsError(t *testing.T) {
	// Known pure-Go grammar fault on this file (slice/variadic near out[1:]...).
	// Packs (language_go.rft) attribute **/*.go → go; no fake PackQueries.
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(root).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	err = w.WalkExtracts(t.Context(), ingest.SourceHop(root, abs), func(*project.FileExtract) bool {
		return true
	})
	if err == nil {
		// Grammar may be fixed upstream; success is fine.
		t.Log("parse succeeded; no fault on this grammar version")
		return
	}
	if !errors.Is(err, ingest.ErrTreeSitterFault) {
		t.Fatalf("want tree-sitter fault error, got %v", err)
	}
}
