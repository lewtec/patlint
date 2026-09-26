package pattern

import (
	"strings"
	"testing"
)

func TestFormatSexpBasic(t *testing.T) {
	p, err := ParseToPat(`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) "," (capture ERR any) ")")`)
	if err != nil {
		t.Fatal(err)
	}
	s := FormatSexp(p)
	if !strings.Contains(s, "(capture F") || !strings.Contains(s, `(ref "go:fmt::Errorf")`) {
		t.Fatalf("sexp=%s", s)
	}
	js, err := FormatSexpJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, `"capture"`) || !strings.Contains(js, `"go:fmt::Errorf"`) {
		t.Fatalf("json=%s", js)
	}
}

func TestFormatSexpRepUnify(t *testing.T) {
	p, err := ParseToPat(`(* (unify c (ref "go:errors::New")))`)
	if err != nil {
		t.Fatal(err)
	}
	s := FormatSexp(p)
	if !strings.Contains(s, "(* ") || !strings.Contains(s, "(unify c") {
		t.Fatalf("sexp=%s", s)
	}
}

func TestFormatSexpOptional(t *testing.T) {
	p, err := ParseToPat(`(? (capture c (ref "go:context::Background")))`)
	if err != nil {
		t.Fatal(err)
	}
	s := FormatSexp(p)
	if !strings.Contains(s, "(alt ") && !strings.Contains(s, "(? ") {
		t.Fatalf("sexp=%s", s)
	}
}
