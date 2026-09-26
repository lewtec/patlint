package ingest_test

import (
	"github.com/lewtec/patlint/pkg/project"
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestPatchFS_WriteOverridesParent(t *testing.T) {
	parent := fstest.MapFS{
		"a.go": {Data: []byte("old")},
		"b.go": {Data: []byte("keep")},
	}
	p := project.NewPatchFS(parent, map[string][]byte{"a.go": []byte("new")})
	got, err := fs.ReadFile(p, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("got %q", got)
	}
	keep, err := fs.ReadFile(p, "b.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(keep) != "keep" {
		t.Fatalf("keep %q", keep)
	}
}

func TestPatchFS_FileRename(t *testing.T) {
	parent := fstest.MapFS{"a.go": {Data: []byte("pkg")}}
	p := project.NewPatchFSLayer(parent, nil, map[string]string{"a.go": "b.go"})
	if _, err := fs.ReadFile(p, "a.go"); err == nil {
		t.Fatal("old name must be gone")
	}
	got, err := fs.ReadFile(p, "b.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pkg" {
		t.Fatalf("got %q", got)
	}
}
