package pattern

import (
	"bytes"
	"errors"
	"github.com/lewtec/patlint/pkg/ingestutil"
)

// ErrUnifyMismatch is returned by UnifySpan.Bind when a later site’s source
// text differs from the first. The NFA treats this as a soft thread reject.
var ErrUnifyMismatch = errors.New("pattern: unify capture mismatch")

// CapSpan is one capture-map value: AppendSpan ($) or UnifySpan (%).
type CapSpan interface {
	Bind(sp ingestutil.Span, source []byte) (CapSpan, error)
	Spans() []ingestutil.Span
}

// AppendSpan is the $ capture discipline: every Bind appends a site.
type AppendSpan []ingestutil.Span

// Bind appends sp and returns the updated value.
func (s AppendSpan) Bind(sp ingestutil.Span, _ []byte) (CapSpan, error) {
	return AppendSpan(append(s, sp)), nil
}

// Spans returns all bound sites.
func (s AppendSpan) Spans() []ingestutil.Span { return s }

// UnifySpan is the % capture discipline: one value for the whole match.
// Later Bind calls require equal source text of the capture span.
type UnifySpan []ingestutil.Span

// Bind sets the first site, or checks text equality against it.
func (s UnifySpan) Bind(sp ingestutil.Span, source []byte) (CapSpan, error) {
	if len(s) == 0 {
		return UnifySpan{sp}, nil
	}
	if !bytes.Equal(s[0].Bytes(source), sp.Bytes(source)) {
		return s, ErrUnifyMismatch
	}
	return s, nil
}

// Spans returns the sole site (len 0 or 1).
func (s UnifySpan) Spans() []ingestutil.Span { return s }

func cloneCapSpan(c CapSpan) CapSpan {
	if c == nil {
		return nil
	}
	sp := c.Spans()
	cp := append([]ingestutil.Span(nil), sp...)
	switch c.(type) {
	case UnifySpan:
		return UnifySpan(cp)
	default:
		return AppendSpan(cp)
	}
}

func flattenCapMap(caps map[string]CapSpan) map[string][]ingestutil.Span {
	if len(caps) == 0 {
		return nil
	}
	out := make(map[string][]ingestutil.Span, len(caps))
	for k, v := range caps {
		if v == nil {
			continue
		}
		out[k] = append([]ingestutil.Span(nil), v.Spans()...)
	}
	return out
}

// bindInto applies Bind for name, constructing AppendSpan or UnifySpan on first write.
func bindInto(caps map[string]CapSpan, name string, sp ingestutil.Span, source []byte, unify bool) error {
	if name == "" || name == "_" || name == "ROOT" {
		return nil
	}
	if cur, ok := caps[name]; ok && cur != nil {
		next, err := cur.Bind(sp, source)
		if err != nil {
			return err
		}
		caps[name] = next
		return nil
	}
	if unify {
		caps[name] = UnifySpan{sp}
	} else {
		caps[name] = AppendSpan{sp}
	}
	return nil
}
