package pattern

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/stretchr/testify/require"
)

func TestRefEmitText(t *testing.T) {
	cases := []struct {
		ref  string
		want string
	}{
		{"go:context::Background", "context.Background"},
		{"@go:context::Background", "context.Background"},
		{"go:net/http::ListenAndServe", "http.ListenAndServe"},
		{"go:fmt::Errorf", "fmt.Errorf"},
		{"go:testing::T", "testing.T"},
	}
	for _, tc := range cases {
		got, err := refEmitText(tc.ref)
		require.NoError(t, err,
			"refEmitText(%q): %v", tc.ref, err)

		if got != tc.want {
			t.Errorf("refEmitText(%q)=%q want %q", tc.ref, got, tc.want)
		}
	}
}

func TestInstantiateEmitRef(t *testing.T) {
	got, err := InstantiateEmit([]any{"ref", "go:context::Background"}, nil, ingestutil.Span{}, Match{})
	require.NoError(t, err)
	require.Equal(t, "context.Background", got,
		"got %q", got)

}
