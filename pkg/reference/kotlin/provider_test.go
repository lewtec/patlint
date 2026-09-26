package kotlinref

import "testing"

func TestResolveImportKnownType(t *testing.T) {
	known := map[string]bool{"demo/Helper.kt": true}
	got := ResolveImport("demo.Helper", known)
	if got != "path:./demo/Helper.kt" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveImportFallback(t *testing.T) {
	got := ResolveImport("kotlin.collections.List", map[string]bool{})
	if got != "kotlin:kotlin.collections.List" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveImportSourceRoot(t *testing.T) {
	known := map[string]bool{"src/main/kotlin/com/foo/Bar.kt": true}
	got := ResolveImport("com.foo.Bar", known)
	if got != "path:./src/main/kotlin/com/foo/Bar.kt" {
		t.Fatalf("got %q", got)
	}
}
