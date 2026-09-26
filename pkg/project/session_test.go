package project

import (
	"testing"
	"testing/fstest"

	"github.com/lewtec/patlint/pkg/sitter"
)

type fakeEng struct{}

func (fakeEng) Parse([]byte, string) (*sitter.Tree, error) { return nil, sitter.ErrUnsupportedLanguage }
func (fakeEng) Has(string) bool                            { return false }
func (fakeEng) Query([]byte, string, string) ([]sitter.QueryMatch, error) {
	return nil, sitter.ErrUnsupportedQuery
}

func TestWithEngineAndFS(t *testing.T) {
	t.Parallel()
	s := NewSession(t.TempDir())
	if s.Engine() != nil {
		t.Fatal("NewSession should not set an engine")
	}
	e := fakeEng{}
	s2 := s.WithEngine(e)
	if s.Engine() != nil {
		t.Fatal("WithEngine mutated the original")
	}
	if s2.Engine() == nil {
		t.Fatal("nil engine after WithEngine")
	}
	s3 := s2.WithFS(fstest.MapFS{})
	if s3.Engine() == nil {
		t.Fatal("WithFS dropped engine")
	}
	if s3.Root != s2.Root {
		t.Fatalf("Root=%q want %q", s3.Root, s2.Root)
	}
}
