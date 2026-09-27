package ingest

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"
)

func TestScoreFlow_NestingAndHybrid(t *testing.T) {
	t.Parallel()
	// unit [0,100)
	// structural [10,40) contains structural [15,20)
	// hybrid [40,70)
	// structural [45,50) inside hybrid — no nest pay on hybrid, but hybrid raises nest
	flows := []project.FlowDef{
		{StartByte: 10, EndByte: 40, Class: project.FlowStructural},
		{StartByte: 15, EndByte: 20, Class: project.FlowStructural},
		{StartByte: 40, EndByte: 70, Class: project.FlowHybrid},
		{StartByte: 45, EndByte: 50, Class: project.FlowStructural},
	}
	rep := ScoreFlow(0, 100, flows)
	require.Equal(t, 6, rep.Score, "incs=%+v", rep.Incs)

	// equal-span flow is not a proper subset of the unit
	rep2 := ScoreFlow(10, 40, []project.FlowDef{
		{StartByte: 10, EndByte: 40, Class: project.FlowStructural},
		{StartByte: 15, EndByte: 20, Class: project.FlowStructural},
	})
	require.Equal(t, 1, rep2.Score, "outer mark excluded; incs=%+v", rep2.Incs)
}

func TestFormatFlowMermaid(t *testing.T) {
	t.Parallel()
	rep := ScoreFlow(0, 100, []project.FlowDef{
		{StartByte: 10, EndByte: 40, Class: project.FlowStructural},
		{StartByte: 15, EndByte: 20, Class: project.FlowStructural},
	})
	out := FormatFlowMermaid("Demo", rep)
	require.Contains(t, out, "flowchart TD")
	require.Contains(t, out, "score=3")
	require.Contains(t, out, "U -->")
}
