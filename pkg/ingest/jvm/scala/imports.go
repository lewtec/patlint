package scala

import "strings"

type scalaImportSpec struct {
	stmt      string
	local     string
	startByte int
	endByte   int
}

func parseScalaImportSpecs(source []byte) []scalaImportSpec {
	text := string(source)
	var specs []scalaImportSpec
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
			stmt := trim
			if i := strings.Index(stmt, "//"); i >= 0 {
				stmt = strings.TrimSpace(stmt[:i])
			}
			body := strings.TrimSpace(strings.TrimPrefix(stmt, "import "))
			if body != "" {
				local := body
				// selectors {A, B => C} — treat whole line as keep-always when braced
				if strings.Contains(body, "{") {
					local = "*"
				} else if strings.HasSuffix(body, "._") || strings.HasSuffix(body, ".*") {
					local = "_"
				} else if i := strings.LastIndex(body, "."); i >= 0 {
					local = body[i+1:]
				}
				end := next
				if nl < 0 {
					end = len(text)
				}
				specs = append(specs, scalaImportSpec{
					stmt:      stmt,
					local:     local,
					startByte: offset,
					endByte:   end,
				})
			}
		}
		if nl < 0 {
			break
		}
		offset = next
	}
	return specs
}
