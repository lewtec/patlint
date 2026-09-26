package report

import (
	"fmt"
	"strings"
)

// Level is a diagnostic severity (SARIF result.level / rule defaultConfiguration.level).
type Level string

const (
	LevelError   Level = "error"
	LevelWarning Level = "warning"
	LevelNote    Level = "note"
)

// String returns the SARIF/wire form (error | warning | note).
func (l Level) String() string { return string(l) }

// ParseLevel normalizes s. Empty means LevelWarning.
// Accepts error, warning, note (case-insensitive).
func ParseLevel(s string) (Level, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return LevelWarning, nil
	}
	l := Level(s)
	switch l {
	case LevelError, LevelWarning, LevelNote:
		return l, nil
	default:
		return "", fmt.Errorf("level %q (want error, warning, or note)", s)
	}
}
