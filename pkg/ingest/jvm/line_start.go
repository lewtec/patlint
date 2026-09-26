package jvm

// PrefixedSpecAtLineStart reports whether pfxAt is preceded only by indentation
// on its line (real import/package statement, not mid-string prose).
func PrefixedSpecAtLineStart(text string, pfxAt int) bool {
	if pfxAt < 0 || pfxAt > len(text) {
		return false
	}
	i := pfxAt
	for i > 0 && text[i-1] != '\n' && text[i-1] != '\r' {
		i--
	}
	for ; i < pfxAt; i++ {
		if text[i] != ' ' && text[i] != '\t' {
			return false
		}
	}
	return true
}
