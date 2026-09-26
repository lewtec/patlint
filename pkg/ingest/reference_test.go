package ingest_test

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingest"
)

func TestParseReference_ExplicitProvider(t *testing.T) {
	ref := ingest.ParseReference("path:./cmd/rft/doc.go::newDocCmd")
	if ref.Provider != "path" || ref.Path != "./cmd/rft/doc.go" || ref.Name != "newDocCmd" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
	if ref.String() != "path:./cmd/rft/doc.go::newDocCmd" {
		t.Fatalf("unexpected string form: %q", ref.String())
	}
}

func TestParseReference_UppercaseProviderCanonicalized(t *testing.T) {
	ref := ingest.ParseReference("Path:./cmd/rft/doc.go::newDocCmd")
	if ref.Provider != "path" || ref.Path != "./cmd/rft/doc.go" || ref.Name != "newDocCmd" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
	if ref.String() != "path:./cmd/rft/doc.go::newDocCmd" {
		t.Fatalf("unexpected canonical string: %q", ref.String())
	}
}

func TestParseReference_ShorthandPathSymbol(t *testing.T) {
	ref := ingest.ParseReference("cmd/rft/doc.go::newDocCmd")
	if ref.Provider != "path" || ref.Path != "./cmd/rft/doc.go" || ref.Name != "newDocCmd" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
	if ref.String() != "path:./cmd/rft/doc.go::newDocCmd" {
		t.Fatalf("unexpected canonical string: %q", ref.String())
	}
}

func TestParseReference_NonPathProvider(t *testing.T) {
	ref := ingest.ParseReference("go:fmt::Println")
	if ref.Provider != "go" || ref.Path != "fmt" || ref.Name != "Println" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
	if ref.String() != "go:fmt::Println" {
		t.Fatalf("unexpected string form: %q", ref.String())
	}
}
