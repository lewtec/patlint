package report

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// WriteFinding writes one human diagnostic line:
// file:line:col: level: ruleId: message
func WriteFinding(w io.Writer, f Finding) error {
	fixNote := ""
	if f.Fixable && f.FixSkipped {
		fixNote = " [fix skipped: overlap]"
	} else if f.Fixable {
		fixNote = " [fixable]"
	}
	_, err := fmt.Fprintf(w, "%s:%d:%d: %s: %s: %s%s\n",
		f.File, f.Line, f.Column, f.Level, f.RuleID, f.Message, fixNote)
	return err
}

// WriteText writes findings as one diagnostic line each.
func WriteText(w io.Writer, findings []Finding) error {
	for _, f := range findings {
		if err := WriteFinding(w, f); err != nil {
			return err
		}
	}
	return nil
}

// WriteTable writes findings as an aligned table.
// Columns: LOCATION  LEVEL  RULE  MESSAGE  FIX
// LOCATION is file:line:col (1-based).
func WriteTable(w io.Writer, findings []Finding) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "LOCATION\tLEVEL\tRULE\tMESSAGE\tFIX"); err != nil {
		return err
	}
	for _, f := range findings {
		fix := ""
		if f.Fixable && f.FixSkipped {
			fix = "skipped"
		} else if f.Fixable {
			fix = "yes"
		}
		loc := fmt.Sprintf("%s:%d:%d", f.File, f.Line, f.Column)
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			loc, f.Level, f.RuleID, f.Message, fix); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// NormalizeFormat accepts text, table, sarif, or rustc (case-insensitive).
// Empty means text.
func NormalizeFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "text":
		return "text", nil
	case "table":
		return "table", nil
	case "sarif":
		return "sarif", nil
	case "rustc":
		return "rustc", nil
	default:
		return "", fmt.Errorf("unknown format %q (want text, table, sarif, or rustc)", format)
	}
}

// WriteFormat dispatches text, table, sarif, or rustc output for lint and run.
func WriteFormat(w io.Writer, format, root string, findings []Finding, rules []Rule) error {
	format, err := NormalizeFormat(format)
	if err != nil {
		return err
	}
	switch format {
	case "table":
		return WriteTable(w, findings)
	case "sarif":
		return WriteSARIF(w, root, findings, rules)
	case "rustc":
		return WriteRustc(w, root, findings)
	default:
		return WriteText(w, findings)
	}
}
