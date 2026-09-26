package pattern

import (
	"testing"
)

func TestNodeToPatSexp(t *testing.T) {
	cases := []struct {
		pat string
	}{
		{`(token "interface{}")`},
		{`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) "," (capture ERR any) ")")`},
		{`(seq (capture F (ref "go:strings::SplitN")) "(" (capture S any) "," (capture SEP any) "," "2" ")")`},
		{`(seq (unify a any) ">" (unify b any) ":" (unify a any) "?" (unify b any))`},
		{`(seq (assert_not_behind (seq (capture NAME (regex "^Err")) "=" (capture PKG any) ".")) (capture E (ref "go:errors::New")) "(" (* any) ")")`},
		{`(seq (token "if") (unify a any) (capture op (regex ">=?")) (unify b any) "{" (token "return") (unify a any) "}" (group (alt (seq (token "return") (unify b any)) (seq (token "else") "{" (token "return") (unify b any) "}"))))`},
		{`(seq (capture F (ref "go:fmt::Errorf")) "(" (* any) (capture E (ref "go:errors::New")) "(" (* any) ")" (* any) ")")`},
	}
	for _, tc := range cases {
		t.Run(tc.pat, func(t *testing.T) {
			n, err := ParsePattern(tc.pat)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			core, err := NodeToPat(n)
			if err != nil {
				t.Fatalf("NodeToPat: %v", err)
			}
			if err := CheckPat(core); err != nil {
				t.Fatalf("CheckPat: %v", err)
			}
			legacy, err := PatToNode(core)
			if err != nil {
				t.Fatalf("PatToNode: %v", err)
			}
			if _, err := compileLegacyNode(legacy); err != nil {
				t.Fatalf("compileLegacyNode: %v", err)
			}
			// Full compile path (bridge inside)
			if _, err := compilePattern(n); err != nil {
				t.Fatalf("compilePattern: %v", err)
			}
		})
	}
}

func TestRepStarCaptureShape(t *testing.T) {
	n, err := ParsePattern(`(* (capture c (ref "go:errors::New")))`)
	if err != nil {
		t.Fatal(err)
	}
	core, err := NodeToPat(n)
	if err != nil {
		t.Fatal(err)
	}
	rep, ok := core.(Rep)
	if !ok {
		t.Fatalf("want Rep, got %T", core)
	}
	if rep.Min != 0 || rep.Max >= 0 {
		t.Fatalf("want unbounded star, got min=%d max=%d", rep.Min, rep.Max)
	}
	cap, ok := rep.Body.(Capture)
	if !ok || cap.Name != "c" {
		t.Fatalf("want Capture c, got %#v", rep.Body)
	}
	if _, ok := cap.Body.(Ref); !ok {
		t.Fatalf("want Ref body, got %T", cap.Body)
	}
}

func TestCheckPatRejectsMixedDiscipline(t *testing.T) {
	p := Seq{Items: []Pat{
		Capture{Name: "a", Body: Any{}},
		Unify{Name: "a", Body: Any{}},
	}}
	if err := CheckPat(p); err == nil {
		t.Fatal("expected discipline error")
	}
}

func TestCheckPatRejectsCaptureAroundRep(t *testing.T) {
	p := Capture{Name: "c", Body: Rep{Min: 0, Max: -1, Body: Any{}}}
	if err := CheckPat(p); err == nil {
		t.Fatal("expected capture-around-rep error")
	}
}
