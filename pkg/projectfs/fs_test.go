package projectfs

import (
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
)

func TestOverlayReadFile(t *testing.T) {
	dir := t.TempDir()
	path := lewpath.New(dir, "a.go").String()
	if err := os.WriteFile(path, []byte("disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := NewOverlay(nil)
	b, err := o.ReadFile(path)
	if err != nil || string(b) != "disk" {
		t.Fatalf("disk read: %q %v", b, err)
	}
	o.SetString(path, "overlay")
	b, err = o.ReadFile(path)
	if err != nil || string(b) != "overlay" {
		t.Fatalf("overlay read: %q %v", b, err)
	}
	o.Delete(path)
	b, err = o.ReadFile(path)
	if err != nil || string(b) != "disk" {
		t.Fatalf("after delete: %q %v", b, err)
	}
}
