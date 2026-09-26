package ingest_test

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingest"
)

func TestResolveReferenceScope_PathDirectory(t *testing.T) {
	tmp := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("cmd", 0755); err != nil {
		t.Fatal(err)
	}

	scope := ingest.ResolveReferenceScope(".", ingest.ParseReference("path:./cmd"))
	if scope.Dir != "cmd" {
		t.Fatalf("unexpected dir: %q", scope.Dir)
	}
	if scope.Reference.Provider != "path" || scope.Reference.Path != "./" {
		t.Fatalf("unexpected normalized ref: %+v", scope.Reference)
	}
}

func TestNormalizeReferenceForScope_PathInsideScope(t *testing.T) {
	dir := t.TempDir()
	file := lewpath.New(dir, "doc.go").String()

	ref := ingest.ParseReference(file + "::newDocCmd")
	norm := ingest.NormalizeReferenceForScope(dir, dir, ref)

	if norm.Provider != "path" || norm.Path != "./doc.go" || norm.Name != "newDocCmd" {
		t.Fatalf("unexpected normalized ref: %+v", norm)
	}
}

func TestNormalizeReferenceForScope_PathOutsideScope(t *testing.T) {
	dir := t.TempDir()
	other := lewpath.New(t.TempDir(), "doc.go").String()

	ref := ingest.ParseReference(other + "::newDocCmd")
	norm := ingest.NormalizeReferenceForScope(dir, dir, ref)

	if norm.Path != ref.Path {
		t.Fatalf("expected unchanged path for outside ref, got %q want %q", norm.Path, ref.Path)
	}
}

func TestAbsolutePathReferenceForScope(t *testing.T) {
	tmp := t.TempDir()
	scope := ingest.ReferenceScope{
		Dir:       tmp,
		Reference: ingest.ParseReference("path:./file.go::name"),
	}
	absRef := ingest.AbsolutePathReferenceForScope(scope)

	want := lewpath.New(tmp, "file.go").String()
	if absRef.Provider != "path" || absRef.Path != want || absRef.Name != "name" {
		t.Fatalf("unexpected absolute ref: %+v", absRef)
	}
}
