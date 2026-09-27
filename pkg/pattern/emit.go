package pattern

import (
	"fmt"
	"github.com/lewtec/patlint/pkg/project"
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
)

// ParseEmit parses a rewrite emit sexpr (SPEC.md Pattern algebra §16.6).
// Atoms are literal text. Lists: (slot), (ref "…"), (seq EMIT…).
func ParseEmit(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%w: emit: empty", ErrRule)
	}
	return ParseSexpData(s)
}

// InstantiateEmit evaluates an emit tree. editSpan is the locus for (slot).
func InstantiateEmit(emit any, source []byte, editSpan ingestutil.Span, m Match) (string, error) {
	switch e := emit.(type) {
	case string:
		return e, nil
	case []any:
		if len(e) == 0 {
			return "", fmt.Errorf("%w: empty emit", ErrRule)
		}
		head, _ := e[0].(string)
		switch head {
		case "slot":
			if len(e) != 1 {
				return "", fmt.Errorf("%w: slot wants arity 0", ErrRule)
			}
			if int(editSpan.EndByte) > len(source) || editSpan.StartByte > editSpan.EndByte {
				return "", fmt.Errorf("%w: slot span out of range", ErrRule)
			}
			return string(source[editSpan.StartByte:editSpan.EndByte]), nil
		case "ref":
			if len(e) != 2 {
				return "", fmt.Errorf("%w: ref emit wants one target", ErrRule)
			}
			s, ok := e[1].(string)
			if !ok {
				return "", fmt.Errorf("%w: ref target must be string", ErrRule)
			}
			return RefEmitText(s)
		case "seq":
			var parts []string
			for _, part := range e[1:] {
				s, err := InstantiateEmit(part, source, editSpan, m)
				if err != nil {
					return "", err
				}
				parts = append(parts, s)
			}
			return ingestutil.EmitSeq(parts, nil), nil
		default:
			return "", fmt.Errorf("%w: unknown emit head %q", ErrRule, head)
		}
	default:
		return "", fmt.Errorf("%w: bad emit type %T", ErrRule, emit)
	}
}

// EmitRefs collects (ref "…") targets from an emit tree (import ensure).
func EmitRefs(emit any) []string {
	var out []string
	var walk func(any)
	walk = func(v any) {
		list, ok := v.([]any)
		if !ok || len(list) == 0 {
			return
		}
		head, _ := list[0].(string)
		if head == "ref" && len(list) == 2 {
			if s, ok := list[1].(string); ok && s != "" {
				out = append(out, strings.TrimPrefix(s, "@"))
			}
			return
		}
		if head == "seq" {
			for _, part := range list[1:] {
				walk(part)
			}
		}
	}
	walk(emit)
	return out
}

func captureText(m Match, name string, source []byte) string {
	sp, ok := m.CaptureFirst(name)
	if !ok {
		return ""
	}
	return sp.Text(source)
}

// EditsForEmit turns matches + emit tree into project.Edit values.
// If take is non-empty, one edit per span of that capture; else the match root.
func EditsForEmit(matches []Match, emit any, source []byte, take string) ([]project.Edit, error) {
	var edits []project.Edit
	for _, m := range matches {
		if take != "" {
			sites := m.Captures[take]
			if len(sites) == 0 {
				continue
			}
			for _, sp := range sites {
				text, err := InstantiateEmit(emit, source, sp, m)
				if err != nil {
					return nil, err
				}
				edits = append(edits, project.Edit{File: m.File, Span: sp, NewText: text})
			}
			continue
		}
		text, err := InstantiateEmit(emit, source, m.Span, m)
		if err != nil {
			return nil, err
		}
		edits = append(edits, project.Edit{File: m.File, Span: m.Span, NewText: text})
	}
	return edits, nil
}
