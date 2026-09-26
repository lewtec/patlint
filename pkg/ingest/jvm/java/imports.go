package java

import "strings"

type javaImportSpec struct {
	stmt      string
	local     string // simple name used by the declaration, or "*" for wildcards
	startByte int    // start of the line in source
	endByte   int    // exclusive end (includes trailing newline when present)
}

// javaImportsNeededByDecl returns full import statements from source whose
// simple names appear in declText.
//
// On-demand / static-star imports (local == "*") are always kept: we cannot
// observe which simple names they supply. That is correct for new-file
// destinations (gson new_module), but when merging into an existing file that
// already has a conflicting on-demand import for the same simple name (e.g.
// dest uses java.awt.List via java.awt.* and we carry java.util.*), the merge
// can make List ambiguous. Prefer single-type imports in moved sources; do not
// try to resolve star conflicts here.
func parseJavaImportSpecs(source []byte) []javaImportSpec {
	text := string(source)
	var specs []javaImportSpec
	offset := 0
	for offset <= len(text) {
		nl := strings.IndexByte(text[offset:], '\n')
		lineEnd := len(text)
		next := len(text)
		if nl >= 0 {
			lineEnd = offset + nl
			next = lineEnd + 1
		}
		line := text[offset:lineEnd]
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "import ") {
			// Drop trailing comments for parsing, keep original line for stmt span.
			stmt := trim
			if i := strings.Index(stmt, "//"); i >= 0 {
				stmt = strings.TrimSpace(stmt[:i])
			}
			if strings.HasSuffix(stmt, ";") {
				body := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(stmt, "import ")), ";"))
				if body != "" {
					local := body
					if strings.HasPrefix(local, "static ") {
						local = strings.TrimSpace(strings.TrimPrefix(local, "static "))
					}
					if strings.HasSuffix(local, ".*") {
						local = "*"
					} else if i := strings.LastIndex(local, "."); i >= 0 {
						local = local[i+1:]
					}
					end := next
					if nl < 0 {
						end = len(text)
					}
					specs = append(specs, javaImportSpec{
						stmt:      stmt,
						local:     local,
						startByte: offset,
						endByte:   end,
					})
				}
			}
		}
		if nl < 0 {
			break
		}
		offset = next
	}
	return specs
}

// ensureJavaImports inserts missing import statements after the package clause.
// each entry in imports is a full "import …;" statement.
