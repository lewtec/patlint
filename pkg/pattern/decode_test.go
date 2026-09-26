package pattern

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodePatRoundTripCLI(t *testing.T) {
	cases := []string{
		`(token "interface{}")`,
		`(seq (capture F (ref "go:fmt::Errorf")) "(" (capture MSG any) "," (capture ERR any) ")")`,
		`(seq (capture F (ref "go:strings::SplitN")) "(" (capture S any) "," (capture SEP any) "," "2" ")")`,
		`(* (unify c (ref "go:errors::New")))`,
		`(? (capture c (ref "go:context::Background")))`,
		`(seq (unify a any) ">" (unify b any) ":" (unify a any) "?" (unify b any))`,
	}
	for _, pat := range cases {
		t.Run(pat, func(t *testing.T) {
			p, err := ParseToPat(pat)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(patToJSON(p))
			if err != nil {
				t.Fatal(err)
			}
			p2, err := DecodePatJSON(raw)
			if err != nil {
				t.Fatalf("decode %s: %v", raw, err)
			}
			s1, s2 := FormatSexp(p), FormatSexp(p2)
			if s1 != s2 {
				t.Fatalf("round-trip\n  got  %s\n  want %s\n  json %s", s2, s1, raw)
			}
		})
	}
}

func TestDecodePatSexpFixtureShape(t *testing.T) {
	raw := []byte(`["token","interface{}"]`)
	p, err := DecodePatJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	tok, ok := p.(Token)
	if !ok || tok.Text != "interface{}" {
		t.Fatalf("got %#v", p)
	}
	n, err := PatToNode(p)
	if err != nil {
		t.Fatal(err)
	}
	if n.Kind != "token" || n.Text != "interface{}" {
		t.Fatalf("node %+v", n)
	}
}

func TestDecodePatRejectsCaptureAroundRep(t *testing.T) {
	// ["capture","c",["*",["any"]]] is rejected by CheckPat
	_, err := DecodePatJSON([]byte(`["capture","c",["*",["any"]]]`))
	if err == nil {
		t.Fatal("want capture-around-rep error")
	}
	if msg := err.Error(); !strings.Contains(msg, "rep") {
		t.Fatalf("want capture-around-rep error, got %v", err)
	}
}

func TestResolvePatternIRFromSexp(t *testing.T) {
	op := Op{
		Mode:        "grep",
		PatternSexp: json.RawMessage(`["token","interface{}"]`),
	}
	if err := op.ResolvePatternIR(); err != nil {
		t.Fatal(err)
	}
	if op.PatternIR.Kind != "token" || op.PatternIR.Text != "interface{}" {
		t.Fatalf("%+v", op.PatternIR)
	}
}

func TestResolvePatternIRPrefersSexp(t *testing.T) {
	op := Op{
		Mode:        "grep",
		PatternIR:   Node{Kind: "lit", Text: "old"},
		PatternSexp: json.RawMessage(`["token","interface{}"]`),
	}
	if err := op.ResolvePatternIR(); err != nil {
		t.Fatal(err)
	}
	if op.PatternIR.Text != "interface{}" {
		t.Fatalf("sexpr should win: %+v", op.PatternIR)
	}
}

func TestCorePat_RejectsPatternIROnly(t *testing.T) {
	op := Op{Mode: "grep", PatternIR: Node{Kind: "token", Text: "x"}}
	if _, err := op.CorePat(); err == nil {
		t.Fatal("want error for pattern_ir-only op")
	}
}
