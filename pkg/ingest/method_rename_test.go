package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"
)

func TestPartitionMethodRename(t *testing.T) {
	result := &project.Result{
		Atoms: []project.Atom{
			{Reference: "path:./a.go::Box.m"},
			{Reference: "path:./a.go::Other.m"},
			{Reference: "path:./a.go::free"},
		},
	}
	scope := PartitionMethodRename(result, []string{"path:./a.go::Box.m"}, "m", "n",
		ingestutil.DottedReceiver, nil)
	require.True(t, scope.Sources.Has("path:./a.go::Box.m"), "sources: %+v", scope.Sources)
	require.True(t, scope.Our.Has("Box"), "our: %+v", scope.Our)
	require.True(t, scope.Foreign.Has("Other"), "foreign: %+v", scope.Foreign)
	require.False(t, scope.Foreign.Has("Box"), "our not foreign")
}
