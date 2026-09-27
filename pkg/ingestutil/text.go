package ingestutil

import "strings"

// AppendDeclText appends declText to content with blank-line separation.
// Ensures content ends with a newline before the blank line when non-empty.
func AppendDeclText(content, declText string) string {
	out := content
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out += "\n"
	}
	if len(out) > 0 {
		out += "\n"
	}
	return out + declText + "\n"
}

// MissingSubstrings returns each needle that does not appear as a substring of
// haystack, preserving needle order and skipping empty needles.
func MissingSubstrings(haystack string, needles []string) []string {
	if len(needles) == 0 {
		return nil
	}
	var missing []string
	for _, s := range needles {
		if s == "" {
			continue
		}
		if !strings.Contains(haystack, s) {
			missing = append(missing, s)
		}
	}
	return missing
}

// JoinLinesNL joins non-empty lines with "\n" and ensures a trailing newline
// when any line is present.
func JoinLinesNL(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// IsCStyleDocCommentLine reports //, block, or javadoc-star comment lines.
func IsCStyleDocCommentLine(line string) bool {
	return strings.HasPrefix(line, "//") ||
		strings.HasPrefix(line, "*") ||
		strings.HasPrefix(line, "/*") ||
		strings.HasPrefix(line, "*/")
}

// IsSlashSlashCommentLine reports // comments only.
func IsSlashSlashCommentLine(line string) bool {
	return strings.HasPrefix(line, "//")
}
