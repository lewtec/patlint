package ignore

import (
	"bufio"
	"io"
	"strings"
)

// attrLine is a raw ignore mark from one .gitattributes line.
type attrLine struct {
	Pattern string
	// Ignore is true when the attribute is set (skip path); false for un-ignore.
	Ignore bool
	Line   int
	Kind   Kind
}

// knownAttrs are .gitattributes attributes that become ignore rules.
// Order is fixed so dual-attr lines emit rules in a stable sequence.
var knownAttrs = []struct {
	name string
	kind Kind
}{
	{"linguist-generated", KindLinguistGenerated},
	{"refactree-ignored", KindRefactreeIgnored},
}

// parseGitAttributesFile reads path and returns linguist-generated /
// refactree-ignored marks only.
func parseGitAttributes(reader io.Reader, path string) ([]attrLine, error) {
	var out []attrLine
	sc := bufio.NewScanner(reader)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pattern := fields[0]
		for _, ka := range knownAttrs {
			set, ok := attrFromAttrs(fields[1:], ka.name)
			if !ok {
				continue
			}
			out = append(out, attrLine{
				Pattern: pattern,
				Ignore:  set,
				Line:    lineNo,
				Kind:    ka.kind,
			})
		}
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// attrFromAttrs reads a boolean-style git attribute by name.
// Bare name → true; -name / !name / name=false → false. Last mention wins.
func attrFromAttrs(attrs []string, want string) (set bool, ok bool) {
	for _, a := range attrs {
		name, val, hasVal := splitAttr(a)
		if name != want {
			continue
		}
		ok = true
		if !hasVal {
			set = true
			continue
		}
		switch strings.ToLower(val) {
		case "true", "yes", "1", "set":
			set = true
		case "false", "no", "0", "unset":
			set = false
		default:
			set = val != ""
		}
	}
	return set, ok
}

func splitAttr(a string) (name, val string, hasVal bool) {
	if a == "" {
		return "", "", false
	}
	switch a[0] {
	case '-':
		return a[1:], "false", true
	case '!':
		return a[1:], "false", true
	}
	if i := strings.IndexByte(a, '='); i >= 0 {
		return a[:i], a[i+1:], true
	}
	return a, "", false
}
