package scalaref

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveImportKnownType(t *testing.T) {
	known := map[string]bool{
		"src/main/scala/com/example/Helper.scala": true,
	}
	got := ResolveImport("com.example.Helper", known)
	want := "path:./src/main/scala/com/example/Helper.scala"
	require.Equal(t, want, got,
		"got %q want %q", got, want)

}

func TestResolveImportUnknown(t *testing.T) {
	got := ResolveImport("scala.collection.mutable", nil)
	require.Equal(t, "scala:scala.collection.mutable", got,
		"got %q", got)

}

func TestNormalizeTypeSpec(t *testing.T) {
	got := normalizeTypeSpec("com/example/Foo")
	require.Equal(t, "com.example.Foo", got,
		"got %q", got)

}
