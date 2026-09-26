package pattern

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
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
		if err != nil {
			t.Fatalf("refEmitText(%q): %v", tc.ref, err)
		}
		if got != tc.want {
			t.Errorf("refEmitText(%q)=%q want %q", tc.ref, got, tc.want)
		}
	}
}

func TestInstantiateEmitRef(t *testing.T) {
	got, err := InstantiateEmit([]any{"ref", "go:context::Background"}, nil, ingestutil.Span{}, Match{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "context.Background" {
		t.Fatalf("got %q", got)
	}
}
