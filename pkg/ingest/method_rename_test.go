package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
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
	if !scope.Sources.Has("path:./a.go::Box.m") || !scope.Our.Has("Box") {
		t.Fatalf("sources/our: %+v %+v", scope.Sources, scope.Our)
	}
	if !scope.Foreign.Has("Other") {
		t.Fatalf("foreign: %+v", scope.Foreign)
	}
	if scope.Foreign.Has("Box") {
		t.Fatal("our not foreign")
	}
}
