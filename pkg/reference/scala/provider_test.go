package scalaref

import "testing"

func TestResolveImportKnownType(t *testing.T) {
	known := map[string]bool{
		"src/main/scala/com/example/Helper.scala": true,
	}
	got := ResolveImport("com.example.Helper", known)
	want := "path:./src/main/scala/com/example/Helper.scala"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveImportUnknown(t *testing.T) {
	got := ResolveImport("scala.collection.mutable", nil)
	if got != "scala:scala.collection.mutable" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeTypeSpec(t *testing.T) {
	if got := normalizeTypeSpec("com/example/Foo"); got != "com.example.Foo" {
		t.Fatalf("got %q", got)
	}
}
