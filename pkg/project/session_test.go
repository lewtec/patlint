package project

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/stretchr/testify/require"
)

type fakeEng struct{}

func (fakeEng) Parse(context.Context, []byte, string) (*sitter.Tree, error) {
	return nil, sitter.ErrUnsupportedLanguage
}
func (fakeEng) Has(context.Context, string) bool { return false }
func (fakeEng) Query(context.Context, []byte, string, string) ([]sitter.QueryMatch, error) {
	return nil, sitter.ErrUnsupportedQuery
}

func TestWithEngineAndFS(t *testing.T) {
	t.Parallel()
	s := NewSession(t.TempDir())
	require.Nil(t, s.Engine(),
		"NewSession should not set an engine")

	e := fakeEng{}
	s2 := s.WithEngine(e)
	require.Nil(t, s.Engine(),
		"WithEngine mutated the original")
	require.NotNil(t, s2.Engine(),
		"nil engine after WithEngine")

	s3 := s2.WithFS(fstest.MapFS{})
	require.NotNil(t, s3.Engine(),
		"WithFS dropped engine")
	require.Equal(t, s2.Root, s3.Root,
		"Root=%q want %q", s3.Root, s2.Root)

}
