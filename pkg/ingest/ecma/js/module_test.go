package js

import (
	"encoding/json"
	"testing"
)

func TestMatchExportPattern(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		pattern string
		request string
		want    string
		ok      bool
	}{
		{
			name:    "single segment capture",
			pattern: "./tsconfigs/*.json",
			request: "./tsconfigs/base.json",
			want:    "base",
			ok:      true,
		},
		{
			name:    "empty capture allowed after slash",
			pattern: "./features/*",
			request: "./features/",
			want:    "",
			ok:      true,
		},
		{
			name:    "empty capture rejected without trailing slash in prefix",
			pattern: "./foo*bar",
			request: "./foobar",
			want:    "",
			ok:      false,
		},
		{
			name:    "prefix mismatch",
			pattern: "./tsconfigs/*.json",
			request: "./other/base.json",
			want:    "",
			ok:      false,
		},
		{
			name:    "suffix mismatch",
			pattern: "./tsconfigs/*.json",
			request: "./tsconfigs/base.js",
			want:    "",
			ok:      false,
		},
		{
			name:    "no star in pattern",
			pattern: "./exact",
			request: "./exact",
			want:    "",
			ok:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := matchExportPattern(tt.pattern, tt.request)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("matchExportPattern(%q, %q) = (%q, %v), want (%q, %v)",
					tt.pattern, tt.request, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestResolveExportsEntry_PatternWithStarReplacement(t *testing.T) {
	t.Parallel()
	exports := json.RawMessage(`{
		"./tsconfigs/*.json": "./configs/*.json",
		"./foo*bar": "./out/*"
	}`)

	got, ok := resolveExportsEntry(exports, "tsconfigs/base.json")
	if !ok {
		t.Fatal("expected pattern match")
	}
	if got != "./configs/base.json" {
		t.Fatalf("got %q, want ./configs/base.json", got)
	}

	if _, ok := resolveExportsEntry(exports, "foobar"); ok {
		t.Fatal("expected empty capture without slash prefix to fail")
	}
}

func TestResolveExportsEntry_TargetWithoutStarUnchanged(t *testing.T) {
	t.Parallel()
	exports := json.RawMessage(`{"./*": "./dist/index.js"}`)
	got, ok := resolveExportsEntry(exports, "anything")
	if !ok {
		t.Fatal("expected pattern match")
	}
	if got != "./dist/index.js" {
		t.Fatalf("got %q, want ./dist/index.js", got)
	}
}
