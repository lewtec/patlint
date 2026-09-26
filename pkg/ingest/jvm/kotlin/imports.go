package kotlin

import "strings"

type kotlinImportSpec struct {
	stmt      string
	local     string
	startByte int
	endByte   int
}

func parseKotlinImportSpecs(source []byte) []kotlinImportSpec {
	text := string(source)
	var specs []kotlinImportSpec
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
				// import foo.Bar as Baz
				if i := strings.Index(body, " as "); i >= 0 {
					local = strings.TrimSpace(body[i+4:])
				} else if strings.HasSuffix(body, ".*") {
					local = "*"
				} else if i := strings.LastIndex(body, "."); i >= 0 {
					local = body[i+1:]
				}
				end := next
				if nl < 0 {
					end = len(text)
				}
				specs = append(specs, kotlinImportSpec{
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
