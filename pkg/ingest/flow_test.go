package ingest

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/pkg/project"
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
	if rep.Score != 1+2+1+2 {
		t.Fatalf("score=%d want 6 incs=%+v", rep.Score, rep.Incs)
	}
	// equal-span flow is not a proper subset of the unit
	rep2 := ScoreFlow(10, 40, []project.FlowDef{
		{StartByte: 10, EndByte: 40, Class: project.FlowStructural},
		{StartByte: 15, EndByte: 20, Class: project.FlowStructural},
	})
	if rep2.Score != 1 {
		t.Fatalf("inner unit score=%d want 1 (outer mark excluded) incs=%+v", rep2.Score, rep2.Incs)
	}
}

func TestFormatFlowMermaid(t *testing.T) {
	t.Parallel()
	rep := ScoreFlow(0, 100, []project.FlowDef{
		{StartByte: 10, EndByte: 40, Class: project.FlowStructural},
		{StartByte: 15, EndByte: 20, Class: project.FlowStructural},
	})
	out := FormatFlowMermaid("Demo", rep)
	if !strings.Contains(out, "flowchart TD") {
		t.Fatalf("missing flowchart: %s", out)
	}
	if !strings.Contains(out, "score=3") {
		t.Fatalf("missing score: %s", out)
	}
	if !strings.Contains(out, "U -->") {
		t.Fatalf("missing edge: %s", out)
	}
}
