package ignore

import (
	"bufio"
	"os"
	"strings"
)

// parseGitignoreFile reads path and returns ignore rules relative to baseDir
// (the directory containing the .gitignore). Last line wins at eval time when
// composed with other sources.
func parseGitignoreFile(path, baseDir string) ([]Rule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rules []Rule
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		// Strip UTF-8 BOM on first line.
		if lineNo == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		r, ok := parseGitignoreLine(line, lineNo, path, baseDir)
		if !ok {
			continue
		}
		rules = append(rules, r)
	}
	if err := sc.Err(); err != nil {
		return rules, err
	}
	return rules, nil
}

func parseGitignoreLine(line string, lineNo int, source, baseDir string) (Rule, bool) {
	// Trailing spaces are significant only when escaped; drop unescaped trail space.
	line = strings.TrimRight(line, "\r")
	if line == "" {
		return Rule{}, false
	}
	// Comment: # at start (after optional leading spaces is not a comment in git —
	// only when # is the first character of the line).
	if line[0] == '#' {
		return Rule{}, false
	}

	negate := false
	if strings.HasPrefix(line, "!") {
		negate = true
		line = line[1:]
		if line == "" {
			return Rule{}, false
		}
	}

	// Unescape common sequences used in patterns (\ , \# , \!).
	line = unescapeGitignore(line)

	dirOnly := strings.HasSuffix(line, "/")
	if dirOnly {
		line = strings.TrimSuffix(line, "/")
	}
	if line == "" {
		return Rule{}, false
	}

	return Rule{
		Pattern: line,
		Negate:  negate,
		DirOnly: dirOnly,
		BaseDir: baseDir,
		Source:  source,
		Line:    lineNo,
		Kind:    KindGitignore,
	}, true
}

func unescapeGitignore(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			b.WriteByte(s[i])
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
