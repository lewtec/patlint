package pattern

import (
	"testing"
)

func TestCoercePatternStringSexp(t *testing.T) {
	p, err := CoercePattern(`(token "interface{}")`, CoerceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.(Token); !ok {
		t.Fatalf("%#v", p)
	}
}

func TestCoercePatternArraySexp(t *testing.T) {
	p, err := CoercePattern([]any{"token", "interface{}"}, CoerceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if tok, ok := p.(Token); !ok || tok.Text != "interface{}" {
		t.Fatalf("%#v", p)
	}
}
