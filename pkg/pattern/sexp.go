package pattern

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FormatSexp formats a Core Pat as a Lisp-style S-expression (whitespace-separated).
// Example: (rep * (capture F (ref "go:fmt::Errorf")))
func FormatSexp(p Pat) string {
	return formatSexp(p)
}

// FormatSexpJSON formats a Core Pat as a JSON array sexpr (lispjson-style).
func FormatSexpJSON(p Pat) (string, error) {
	v := patToJSON(p)
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func formatSexp(p Pat) string {
	switch x := p.(type) {
	case Any:
		return "any"
	case Lit:
		return strconv.Quote(x.Text)
	case Token:
		return fmt.Sprintf("(token %s)", strconv.Quote(x.Text))
	case Regex:
		parts := []string{"regex", strconv.Quote(x.RE)}
		if x.Invert {
			return fmt.Sprintf("(not (%s))", strings.Join(append([]string{"regex", strconv.Quote(x.RE)}, regexExtras(x)...), " "))
		}
		if x.Equals != "" && x.RE == "" {
			return fmt.Sprintf("(equals %s)", strconv.Quote(x.Equals))
		}
		extra := regexExtras(x)
		if len(extra) == 0 {
			return fmt.Sprintf("(%s)", strings.Join(parts, " "))
		}
		return fmt.Sprintf("(%s)", strings.Join(append(parts, extra...), " "))
	case Ref:
		return fmt.Sprintf("(ref %s)", strconv.Quote(x.Target))
	case Not:
		return fmt.Sprintf("(not %s)", formatSexp(x.Inner))
	case Seq:
		if len(x.Items) == 0 {
			return "(seq)"
		}
		var b strings.Builder
		b.WriteString("(seq")
		for _, it := range x.Items {
			b.WriteByte(' ')
			b.WriteString(formatSexp(it))
		}
		b.WriteByte(')')
		return b.String()
	case Alt:
		var b strings.Builder
		b.WriteString("(alt")
		for _, it := range x.Items {
			b.WriteByte(' ')
			b.WriteString(formatSexp(it))
		}
		b.WriteByte(')')
		return b.String()
	case Rep:
		if x.LocalOptional {
			return fmt.Sprintf("(? %s)", formatSexp(x.Body))
		}
		if x.Min == 0 && x.Max < 0 {
			return fmt.Sprintf("(* %s)", formatSexp(x.Body))
		}
		if x.Min == 1 && x.Max < 0 {
			return fmt.Sprintf("(+ %s)", formatSexp(x.Body))
		}
		q := repQuantAtom(x.Min, x.Max)
		return fmt.Sprintf("(rep %s %s)", q, formatSexp(x.Body))
	case Group:
		return fmt.Sprintf("(group %s)", formatSexp(x.Body))
	case AssertNotBehind:
		return fmt.Sprintf("(assert_not_behind %s)", formatSexp(x.Body))
	case Capture:
		return fmt.Sprintf("(capture %s %s)", x.Name, formatSexp(x.Body))
	case Unify:
		return fmt.Sprintf("(unify %s %s)", x.Name, formatSexp(x.Body))
	default:
		return fmt.Sprintf("(unknown %T)", p)
	}
}

func regexExtras(x Regex) []string {
	var extra []string
	if x.CaptureGroup != 0 {
		extra = append(extra, strconv.Itoa(x.CaptureGroup))
	}
	if x.FromCapture != "" {
		extra = append(extra, strconv.Quote(x.FromCapture))
	}
	return extra
}

func repQuantAtom(min, max int) string {
	if min == 0 && max < 0 {
		return "*"
	}
	if min == 1 && max < 0 {
		return "+"
	}
	if min == 0 && max == 1 {
		return "?"
	}
	if max < 0 {
		return fmt.Sprintf("%d..", min)
	}
	if min == max {
		return strconv.Itoa(min)
	}
	return fmt.Sprintf("%d..%d", min, max)
}

func patToJSON(p Pat) any {
	switch x := p.(type) {
	case Any:
		return []any{"any"}
	case Lit:
		return x.Text
	case Token:
		return []any{"token", x.Text}
	case Regex:
		if x.Invert {
			return []any{"not", []any{"regex", x.RE}}
		}
		if x.Equals != "" && x.RE == "" {
			return []any{"equals", x.Equals}
		}
		out := []any{"regex", x.RE}
		if x.CaptureGroup != 0 {
			out = append(out, x.CaptureGroup)
		}
		if x.FromCapture != "" {
			out = append(out, x.FromCapture)
		}
		return out
	case Ref:
		return []any{"ref", x.Target}
	case Not:
		return []any{"not", patToJSON(x.Inner)}
	case Seq:
		out := make([]any, 0, 1+len(x.Items))
		out = append(out, "seq")
		for _, it := range x.Items {
			out = append(out, patToJSON(it))
		}
		return out
	case Alt:
		out := make([]any, 0, 1+len(x.Items))
		out = append(out, "alt")
		for _, it := range x.Items {
			out = append(out, patToJSON(it))
		}
		return out
	case Rep:
		if x.LocalOptional {
			return []any{"?", patToJSON(x.Body)}
		}
		if x.Min == 0 && x.Max < 0 {
			return []any{"*", patToJSON(x.Body)}
		}
		if x.Min == 1 && x.Max < 0 {
			return []any{"+", patToJSON(x.Body)}
		}
		if x.Max < 0 {
			return []any{"rep", x.Min, patToJSON(x.Body)}
		}
		if x.Min == x.Max {
			return []any{"rep", x.Min, patToJSON(x.Body)}
		}
		return []any{"rep", x.Min, x.Max, patToJSON(x.Body)}
	case Group:
		return []any{"group", patToJSON(x.Body)}
	case AssertNotBehind:
		return []any{"assert_not_behind", patToJSON(x.Body)}
	case Capture:
		return []any{"capture", x.Name, patToJSON(x.Body)}
	case Unify:
		return []any{"unify", x.Name, patToJSON(x.Body)}
	default:
		return []any{"unknown", fmt.Sprintf("%T", p)}
	}
}

// ParseToPat parses a Core sexpr pattern string (same as ParseSexp).
func ParseToPat(s string) (Pat, error) {
	return ParseSexp(s)
}
