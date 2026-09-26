package pattern

import (
	"strings"
	"testing"
)

func TestFormatNFAFromPat_Lit(t *testing.T) {
	p, err := ParseToPat(`(token "interface{}")`)
	if err != nil {
		t.Fatal(err)
	}
	s, err := FormatNFAFromPat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "START") || !strings.Contains(s, "ACCEPT") {
		t.Fatalf("missing start/accept: %s", s)
	}
	if !strings.Contains(s, `token "interface{}"`) {
		t.Fatalf("missing consume: %s", s)
	}
}

func TestFormatCompiledPlan_Under(t *testing.T) {
	m, err := ParseMatcher(`(under (token "func") (token "x"))`)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	s := FormatCompiledPlan(cm)
	if !strings.Contains(s, "under") || !strings.Contains(s, "region:") || !strings.Contains(s, "body:") {
		t.Fatalf("plan dump: %s", s)
	}
	if !strings.Contains(s, "ε-NFA") {
		t.Fatalf("want leaf NFAs: %s", s)
	}
}

func TestFormatNFADot(t *testing.T) {
	p := Lit{Text: "if"}
	dot, err := FormatNFADot(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dot, "digraph") || !strings.Contains(dot, "lit") {
		t.Fatalf("%s", dot)
	}
}
