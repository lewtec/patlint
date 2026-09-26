package ingest

import "testing"

func TestApplyDeclWrap(t *testing.T) {
	got := applyDeclarationWrap("type", "Config struct {\n\t\tName string\n\t}", "\t")
	want := "type Config struct {\n\tName string\n}"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if applyDeclarationWrap("var", "ErrNoGoVersions   = ErrNoVersions", "\t") != "var ErrNoGoVersions   = ErrNoVersions" {
		t.Fatal("var wrap")
	}
}

func TestFirstIdentToken(t *testing.T) {
	src := []byte("type ( A; B )")
	if got := firstIdentifierToken(src, 0, uint32(len(src))); got != "type" {
		t.Fatalf("got %q", got)
	}
	if got := firstIdentifierToken([]byte("  const (\n\tC = iota\n)"), 0, 20); got != "const" {
		t.Fatalf("got %q", got)
	}
}
