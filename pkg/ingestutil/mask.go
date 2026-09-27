package ingestutil

// MaskNonNewlinesInPlace replaces bytes in [start, end) with spaces, keeping
// newlines so residual whole-word scans still see line structure. Indices are
// clamped to buf.
func MaskNonNewlinesInPlace(buf []byte, start, end int) {
	if start < 0 {
		start = 0
	}
	if end > len(buf) {
		end = len(buf)
	}
	for i := start; i < end; i++ {
		if buf[i] != '\n' {
			buf[i] = ' '
		}
	}
}
