// Package testutil holds small helpers shared by package tests.
package testutil

import (
	"os"
	"path/filepath"

	"github.com/lewtec/patlint/pkg/projectfs"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
)

// ModuleRoot walks up from the working directory to the nearest go.mod.
func ModuleRoot(t testing.TB) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := (projectfs.OS{}).Stat(lewpath.New(wd, "go.mod").String()); err == nil {
			return wd
		}
		p := filepath.Dir(wd)
		if p == wd {
			t.Fatal("no go.mod above " + wd)
		}
		wd = p
	}
}
