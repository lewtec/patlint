package reference

import "testing"

func TestLastPathComponent(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"foo", "foo"},
		{"a/b/c", "c"},
		{"pkg/foo/", "foo"},
		{"/abs", "abs"},
	}
	for _, tc := range cases {
		if got := LastPathComponent(tc.in); got != tc.want {
			t.Errorf("LastPathComponent(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestJoinProviderPath(t *testing.T) {
	cases := []struct{ base, name, want string }{
		{"", "a", "a"},
		{"a", "", "a"},
		{"a/b", "c", "a/b/c"},
		{"/a/", "/b/", "a/b"},
	}
	for _, tc := range cases {
		if got := JoinProviderPath(tc.base, tc.name); got != tc.want {
			t.Errorf("JoinProviderPath(%q,%q)=%q want %q", tc.base, tc.name, got, tc.want)
		}
	}
}

func TestParentProviderPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"fmt", ""},
		{"net/http", "net"},
		{"github.com/lewtec/patlint/cmd/rft", "github.com/lewtec/patlint/cmd"},
		{"", ""},
		{"/a/b/", "a"},
	}
	for _, tc := range cases {
		if got := ParentProviderPath(tc.in); got != tc.want {
			t.Errorf("ParentProviderPath(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
