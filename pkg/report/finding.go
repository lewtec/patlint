// Package report is the shared diagnostic output spine for rft run and related commands.
// Engines emit Finding values; CLI and tests call WriteFormat / WriteFinding / WriteSARIF.
package report

import (
	"github.com/lewtec/patlint/pkg/project"
)

// Finding is one diagnostic row for text, table, rustc, and SARIF.
type Finding struct {
	RuleID  string
	Level   Level
	Message string
	File    string
	Line    int // 1-based
	Column  int // 1-based
	EndLine int
	EndCol  int
	Snippet string
	// SiteEdits are replacement edits for this match (SARIF result.fixes).
	SiteEdits []project.Edit
	// Fixable is true when site edits were produced.
	Fixable bool
	// FixSkipped is true when Fixable but edits were dropped due to conflict.
	FixSkipped bool
	// Source is optional file bytes at match time (for SARIF fix regions).
	Source []byte
}

// Rule is a reporting descriptor for SARIF tool.driver.rules.
type Rule struct {
	ID      string
	Message string
	Level   Level
}
