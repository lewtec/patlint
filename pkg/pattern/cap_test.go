package pattern

import (
	"errors"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"testing"
)

func TestAppendSpanBind(t *testing.T) {
	var c CapSpan = AppendSpan{}
	c, err := c.Bind(ingestutil.Span{StartByte: 0, EndByte: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err = c.Bind(ingestutil.Span{StartByte: 2, EndByte: 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sp := c.Spans()
	if len(sp) != 2 || sp[0].StartByte != 0 || sp[1].StartByte != 2 {
		t.Fatalf("spans=%v", sp)
	}
}

func TestUnifySpanBind(t *testing.T) {
	src := []byte("xx yy xx")
	a := ingestutil.Span{StartByte: 0, EndByte: 2} // "xx"
	b := ingestutil.Span{StartByte: 6, EndByte: 8} // "xx"
	y := ingestutil.Span{StartByte: 3, EndByte: 5} // "yy"

	var c CapSpan = UnifySpan{}
	c, err := c.Bind(a, src)
	if err != nil {
		t.Fatal(err)
	}
	c, err = c.Bind(b, src)
	if err != nil {
		t.Fatalf("equal text should unify: %v", err)
	}
	if len(c.Spans()) != 1 {
		t.Fatalf("len=%d", len(c.Spans()))
	}
	_, err = c.Bind(y, src)
	if !errors.Is(err, ErrUnifyMismatch) {
		t.Fatalf("want ErrUnifyMismatch, got %v", err)
	}
}

func TestBindIntoFirstAndDiscipline(t *testing.T) {
	src := []byte("ab")
	caps := map[string]CapSpan{}
	if err := bindInto(caps, "a", ingestutil.Span{0, 1}, src, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := caps["a"].(UnifySpan); !ok {
		t.Fatalf("type %T", caps["a"])
	}
	if err := bindInto(caps, "a", ingestutil.Span{0, 1}, src, true); err != nil {
		t.Fatal(err)
	}
	// second name append
	if err := bindInto(caps, "b", ingestutil.Span{1, 2}, src, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := caps["b"].(AppendSpan); !ok {
		t.Fatalf("type %T", caps["b"])
	}
	if err := bindInto(caps, "b", ingestutil.Span{1, 2}, src, false); err != nil {
		t.Fatal(err)
	}
	if len(caps["b"].Spans()) != 2 {
		t.Fatalf("append len=%d", len(caps["b"].Spans()))
	}
}
