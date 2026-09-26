package pattern

import (
	"testing"
)

func TestParseSexpRoundTripFormat(t *testing.T) {
	cases := []string{
		`(token "interface{}")`,
		`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) "," (capture ERR any) ")")`,
		`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG (regex "(?i)^failed to\\s+(.*)" 1)) "," (capture ERR any) ")")`,
		`(seq (capture blank (regex "^_$")) (capture op (regex "^(:=|=)$" 1)) (group (seq (capture f (regex "^[A-Za-z_]")) "(" (* any) ")")))`,
		`(* (unify c (ref "go:errors::New")))`,
		`(? (capture c (ref "go:context::Background")))`,
		`(seq (unify a any) ">" (unify b any) ":" (unify a any) "?" (unify b any))`,
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			p, err := ParseToPat(src)
			if err != nil {
				t.Fatal(err)
			}
			// ParseSexp runs Expand: numeric/?/+ rep macros expand. Round-trip is
			// stable after expand (Format → ParseSexp → Format), not identity with
			// pre-expand IR (e.g. LocalOptional → (alt (seq) body)).
			lisp := FormatSexp(p)
			p2, err := ParseSexp(lisp)
			if err != nil {
				t.Fatalf("ParseSexp(%q): %v", lisp, err)
			}
			p3, err := ParseSexp(FormatSexp(p2))
			if err != nil {
				t.Fatalf("re-ParseSexp: %v", err)
			}
			if FormatSexp(p2) != FormatSexp(p3) {
				t.Fatalf("expand not stable\n  once  %s\n  twice %s", FormatSexp(p2), FormatSexp(p3))
			}
		})
	}
}

func TestParseToPatLispString(t *testing.T) {
	p, err := ParseToPat(`(token "interface{}")`)
	if err != nil {
		t.Fatal(err)
	}
	tok, ok := p.(Token)
	if !ok || tok.Text != "interface{}" {
		t.Fatalf("%#v", p)
	}
}

func TestParseToPatBareAtom(t *testing.T) {
	p, err := ParseToPat(`interface{}`)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := p.(Lit); !ok || got.Text != "interface{}" {
		t.Fatalf("%#v", p)
	}
}
