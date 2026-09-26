package ingestutil

import "strings"

// IsIdentChar reports whether b is an ASCII letter, digit, or underscore.
func IsIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// IsIdentCharJava is IsIdentChar plus '$' (Java/JS identifiers).
func IsIdentCharJava(b byte) bool {
	return IsIdentChar(b) || b == '$'
}

// IdentUsed reports whether ident appears as a whole identifier in text.
// isIdent classifies identifier characters (IsIdentChar or IsIdentCharJava).
// Empty ident or a nil isIdent always returns false.
func IdentUsed(text, ident string, isIdent func(byte) bool) bool {
	if ident == "" || isIdent == nil {
		return false
	}
	off := 0
	for {
		idx := strings.Index(text[off:], ident)
		if idx < 0 {
			return false
		}
		pos := off + idx
		end := pos + len(ident)
		if pos > 0 && isIdent(text[pos-1]) {
			off = end
			continue
		}
		if end < len(text) && isIdent(text[end]) {
			off = end
			continue
		}
		return true
	}
}

// IdentUsedJava is IdentUsed with Java/JS identifier characters (includes $).
func IdentUsedJava(text, ident string) bool {
	return IdentUsed(text, ident, IsIdentCharJava)
}

// DottedReceiver returns the qualifier before the last "." in symbol
// (e.g. "Outer.Inner.method" → "Outer.Inner"). ok is false when there is no dot
// segment pair.
func DottedReceiver(symbol string) (string, bool) {
	if symbol == "" {
		return "", false
	}
	i := strings.LastIndex(symbol, ".")
	if i <= 0 {
		return "", false
	}
	recv := symbol[:i]
	return recv, recv != ""
}
