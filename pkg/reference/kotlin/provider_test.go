package kotlinref

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveImportKnownType(t *testing.T) {
	known := map[string]bool{"demo/Helper.kt": true}
	got := ResolveImport("demo.Helper", known)
	require.Equal(t, "path:./demo/Helper.kt", got,
		"got %q", got)

}

func TestResolveImportFallback(t *testing.T) {
	got := ResolveImport("kotlin.collections.List", map[string]bool{})
	require.Equal(t, "kotlin:kotlin.collections.List", got,
		"got %q", got)

}

func TestResolveImportSourceRoot(t *testing.T) {
	known := map[string]bool{"src/main/kotlin/com/foo/Bar.kt": true}
	got := ResolveImport("com.foo.Bar", known)
	require.Equal(t, "path:./src/main/kotlin/com/foo/Bar.kt", got,
		"got %q", got)

}
