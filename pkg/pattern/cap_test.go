package pattern

import (
	"testing"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/stretchr/testify/require"
)

func TestAppendSpanBind(t *testing.T) {
	var c CapSpan = AppendSpan{}
	c, err := c.Bind(ingestutil.Span{StartByte: 0, EndByte: 1}, nil)
	require.NoError(t, err)

	c, err = c.Bind(ingestutil.Span{StartByte: 2, EndByte: 3}, nil)
	require.NoError(t, err)

	sp := c.Spans()
	require.False(t, len(sp) != 2 || sp[0].StartByte != 0 || sp[1].StartByte != 2,
		"spans=%v", sp)

}

func TestUnifySpanBind(t *testing.T) {
	src := []byte("xx yy xx")
	a := ingestutil.Span{StartByte: 0, EndByte: 2} // "xx"
	b := ingestutil.Span{StartByte: 6, EndByte: 8} // "xx"
	y := ingestutil.Span{StartByte: 3, EndByte: 5} // "yy"

	var c CapSpan = UnifySpan{}
	c, err := c.Bind(a, src)
	require.NoError(t, err)

	c, err = c.Bind(b, src)
	require.NoError(t, err,
		"equal text should unify: %v", err)
	require.Len(t, c.Spans(), 1,
		"len=%d", len(c.Spans()))

	_, err = c.Bind(y, src)
	require.ErrorIs(t, err, ErrUnifyMismatch,
		"want ErrUnifyMismatch, got %v", err)

}

func TestBindIntoFirstAndDiscipline(t *testing.T) {
	src := []byte("ab")
	caps := map[string]CapSpan{}
	{
		err := bindInto(caps, "a", ingestutil.Span{0, 1}, src, true)
		require.NoError(t, err)
	}
	{

		_, ok := caps["a"].(UnifySpan)
		require.True(t, ok,
			"type %T", caps["a"])
	}
	{

		err := bindInto(caps, "a", ingestutil.Span{0, 1}, src, true)
		require.NoError(t, err)
	}
	{

		// second name append
		err := bindInto(caps, "b", ingestutil.Span{1, 2}, src, false)
		require.NoError(t, err)
	}

	_, ok := caps["b"].(AppendSpan)
	require.True(t, ok,
		"type %T", caps["b"])

	err := bindInto(caps, "b", ingestutil.Span{1, 2}, src, false)
	require.NoError(t, err)
	require.Len(t, caps["b"].Spans(), 2,
		"append len=%d", len(caps["b"].Spans()))

}
