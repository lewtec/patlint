package ingest

import "github.com/lewtec/patlint/pkg/ingestutil"

import "testing"

func TestChildByTypeNil(t *testing.T) {
	if ingestutil.ChildByType(nil, "identifier") != nil {
		t.Fatal("expected nil for nil node")
	}
}

func TestChildByFieldNil(t *testing.T) {
	if ingestutil.ChildByField(nil, "name") != nil {
		t.Fatal("expected nil for nil node")
	}
}
