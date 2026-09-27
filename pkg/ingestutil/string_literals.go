package ingestutil

// ForEachStringLiteral walks double-quoted and raw backtick string literals in
// content. lit includes the opening and closing delimiters; start is the byte
// offset of the opening quote. Iteration stops early if fn returns false.
//
// Escape handling for double-quoted strings matches Go/JSON-style \" and \\
// pairs; raw strings end at the next unescaped backtick (no escapes).
//
// // line comments and /* block comments */ are skipped so quotes inside them
// do not open false literals (e.g. Go docs mentioning " in a // comment).
// Single-quoted runes/chars are also skipped so '"'-style quotes do not start
// a double-quoted scan.
func ForEachStringLiteral(content []byte, fn func(lit string, start int) bool) {
	text := string(content)
	i := 0
	for i < len(text) {
		c := text[i]
		// // line comment
		if c == '/' && i+1 < len(text) && text[i+1] == '/' {
			i += 2
			for i < len(text) && text[i] != '\n' {
				i++
			}
			continue
		}
		// /* block comment */
		if c == '/' && i+1 < len(text) && text[i+1] == '*' {
			i += 2
			for i+1 < len(text) && !(text[i] == '*' && text[i+1] == '/') {
				i++
			}
			if i+1 < len(text) {
				i += 2
			} else {
				return
			}
			continue
		}
		// 'x' / '\n' / '"'-style rune or char literal
		if c == '\'' {
			i++
			for i < len(text) {
				if text[i] == '\\' && i+1 < len(text) {
					i += 2
					continue
				}
				if text[i] == '\'' {
					i++
					break
				}
				if text[i] == '\n' {
					break
				}
				i++
			}
			continue
		}
		if c == '"' {
			start := i
			i++
			end := -1
			for i < len(text) {
				if text[i] == '\\' && i+1 < len(text) {
					i += 2
					continue
				}
				if text[i] == '"' {
					end = i
					break
				}
				i++
			}
			if end < 0 {
				return
			}
			if !fn(text[start:end+1], start) {
				return
			}
			i = end + 1
			continue
		}
		if c == '`' {
			start := i
			j := i + 1
			for j < len(text) && text[j] != '`' {
				j++
			}
			if j >= len(text) {
				return
			}
			if !fn(text[start:j+1], start) {
				return
			}
			i = j + 1
			continue
		}
		i++
	}
}
