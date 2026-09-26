package ingestutil

import "testing"

func TestDottedReceiver(t *testing.T) {
	if _, ok := DottedReceiver(""); ok {
		t.Fatal("empty")
	}
	if _, ok := DottedReceiver("plain"); ok {
		t.Fatal("no dot")
	}
	recv, ok := DottedReceiver("Class.method")
	if !ok || recv != "Class" {
		t.Fatalf("got %q %v", recv, ok)
	}
	recv, ok = DottedReceiver("Outer.Inner.m")
	if !ok || recv != "Outer.Inner" {
		t.Fatalf("nested: %q", recv)
	}
}

func TestIdentUsedJava(t *testing.T) {
	if !IdentUsedJava("$foo bar", "$foo") {
		t.Fatal("want hit")
	}
	if IdentUsedJava("a$foo", "$foo") {
		t.Fatal("mid-ident")
	}
}

func TestMissingSubstrings(t *testing.T) {
	got := MissingSubstrings("import A\n", []string{"import A", "import B", ""})
	if len(got) != 1 || got[0] != "import B" {
		t.Fatalf("%v", got)
	}
}

func TestIsCStyleDocCommentLine(t *testing.T) {
	if !IsCStyleDocCommentLine("// x") || !IsCStyleDocCommentLine("* x") {
		t.Fatal("cstyle")
	}
	if IsSlashSlashCommentLine("* x") {
		t.Fatal("slash only")
	}
}
