package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lewtec/lewkit/x/text/report"
	"github.com/stretchr/testify/require"
)

func TestFindingSinkStreamsTextBeforeClose(t *testing.T) {
	var buf bytes.Buffer
	sink := newFindingSink(report.FormatText, &buf, "")
	first := report.Finding{RuleID: "a", Level: report.LevelWarning, Message: "one", File: "a.go", Line: 1, Column: 1}
	require.NoError(t, sink.Write(first))
	require.Contains(t, buf.String(), "a.go:1:1: warning: a: one")
	require.NoError(t, sink.Write(report.Finding{RuleID: "b", Level: report.LevelError, Message: "two", File: "b.go", Line: 2, Column: 3}))
	require.NoError(t, sink.Close(nil))
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	require.Equal(t, []string{
		"a.go:1:1: warning: a: one",
		"b.go:2:3: error: b: two",
	}, lines)
}

func TestFindingSinkHoldsSARIFUntilClose(t *testing.T) {
	var buf bytes.Buffer
	sink := newFindingSink(report.FormatSARIF, &buf, "")
	require.NoError(t, sink.Write(report.Finding{RuleID: "a", Level: report.LevelWarning, Message: "one", File: "a.go", Line: 1, Column: 1}))
	require.Empty(t, buf.String())
	require.NoError(t, sink.Close([]report.Rule{{ID: "a", Message: "one", Level: report.LevelWarning}}))
	require.Contains(t, buf.String(), `"name": "patlint"`)
	require.Contains(t, buf.String(), `"ruleId": "a"`)
}
