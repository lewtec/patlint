package ingestutil

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// EmitSeq is rewrite emit for a token seq: fill path/leaf/qual/pkg, then join
// grammar trivia (word space, quote parity, attach punct). No trailing newline.
func EmitSeq(tokens []string, fields map[string]string) string {
	if len(tokens) == 0 {
		return ""
	}
	var out []string
	for _, t := range tokens {
		if fields != nil {
			if v, ok := fields[t]; ok {
				t = v
			}
		}
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	return joinGrammarTokens(out)
}

func joinGrammarTokens(toks []string) string {
	if len(toks) == 0 {
		return ""
	}
	var b strings.Builder
	prev := ""
	openQuote := map[rune]bool{}
	for _, t := range toks {
		if t == "" {
			continue
		}
		if prev != "" && tokenSpace(prev, t, openQuote) {
			b.WriteByte(' ')
		}
		if q, ok := quoteRune(t); ok {
			openQuote[q] = !openQuote[q]
		}
		b.WriteString(t)
		prev = t
	}
	return b.String()
}

func tokenSpace(a, b string, openQuote map[rune]bool) bool {
	if q, ok := quoteRune(b); ok {
		if !openQuote[q] {
			return isWordToken(a)
		}
		return false
	}
	if _, ok := quoteRune(a); ok {
		return false
	}
	if attachRight(a) || attachLeft(b) {
		return false
	}
	return true
}

func quoteRune(s string) (rune, bool) {
	if s == `"` || s == "'" || s == "`" {
		r, _ := utf8.DecodeRuneInString(s)
		return r, true
	}
	return 0, false
}

func isWordToken(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '@' || r == '#'
}

func attachRight(s string) bool {
	if s == "" {
		return false
	}
	switch s[len(s)-1] {
	case '(', '[', '#':
		return true
	}
	return len(s) > 1 && strings.HasSuffix(s, "-")
}

func attachLeft(s string) bool {
	if s == "" {
		return false
	}
	switch s[0] {
	case ')', ']', '(', '[', ',', ';':
		return true
	}
	if s == "." {
		return true
	}
	return len(s) > 1 && strings.HasPrefix(s, "-")
}
