package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/stretchr/testify/require"
)

func TestChildByTypeNil(t *testing.T) {
	require.Nil(t, ingestutil.ChildByType(nil, "identifier"),
		"expected nil for nil node")

}

func TestChildByFieldNil(t *testing.T) {
	require.Nil(t, ingestutil.ChildByField(nil, "name"),
		"expected nil for nil node")

}
