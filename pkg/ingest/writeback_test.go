package ingest_test

import (
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestWriteBack_LastWriteWins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "a.go").String(), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent := fstest.MapFS{"a.go": {Data: []byte("one")}}
	mid := project.NewPatchFS(parent, map[string][]byte{"a.go": []byte("two")})
	top := project.NewPatchFS(mid, map[string][]byte{"a.go": []byte("three")})
	sess := &project.Session{Root: dir, FS: top}
	if err := ingest.WriteBack(t.Context(), sess); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(lewpath.New(dir, "a.go").String())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "three" {
		t.Fatalf("got %q", got)
	}
}

func TestWriteBack_EmptyNoop(t *testing.T) {
	dir := t.TempDir()
	sess := project.NewSession(dir).WithEngine(ccgo.Engine{})
	if err := ingest.WriteBack(t.Context(), sess); err != nil {
		t.Fatal(err)
	}
}

func TestWriteBack_FileRename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(lewpath.New(dir, "a.go").String(), []byte("pkg"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent := os.DirFS(dir)
	layer := project.NewPatchFSLayer(parent, nil, map[string]string{"a.go": "b.go"})
	sess := &project.Session{Root: dir, FS: layer}
	if err := ingest.WriteBack(t.Context(), sess); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lewpath.New(dir, "a.go").String()); !os.IsNotExist(err) {
		t.Fatal("old file must be gone")
	}
	got, err := os.ReadFile(lewpath.New(dir, "b.go").String())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pkg" {
		t.Fatalf("got %q", got)
	}
}

func TestOverlayWrites_NonPatchEmpty(t *testing.T) {
	if n := len(project.OverlayWrites(fstest.MapFS{"a": {Data: []byte("x")}})); n != 0 {
		t.Fatalf("n=%d", n)
	}
	_ = fs.FS(nil)
}
