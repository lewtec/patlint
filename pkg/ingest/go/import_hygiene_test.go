package ingestgo

import (
	"testing"
)

func TestGoAssumedImportName(t *testing.T) {
	cases := map[string]string{
		"fmt":                             "fmt",
		"gopkg.in/yaml.v3":                "yaml",
		"github.com/pelletier/go-toml/v2": "toml",
		"github.com/testcontainers/testcontainers-go": "testcontainers",
		"golang.org/x/sync/errgroup":                  "errgroup",
	}
	for path, want := range cases {
		if got := goAssumedImportName(path); got != want {
			t.Errorf("goAssumedImportName(%q)=%q want %q", path, got, want)
		}
	}
}
