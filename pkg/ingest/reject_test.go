package ingest

import "testing"

func TestMemberReceiver(t *testing.T) {
	cases := map[string]string{
		"Session.Close":    "Session",
		"*Session.Close":   "Session",
		"Set[T].Add":       "Set",
		"*Set[T].Add":      "Set",
		"SudoCommand.Slug": "SudoCommand",
		"Helper":           "",
	}
	for name, want := range cases {
		if got := memberReceiver(name); got != want {
			t.Fatalf("%s: got %q want %q", name, got, want)
		}
	}
}

func TestIdentExported(t *testing.T) {
	if !identifierExported("Helper") || identifierExported("helper") || identifierExported("createWorkspacedShim") {
		t.Fatal("export")
	}
}

func TestTestFile(t *testing.T) {
	if !testFile("pkga/a_test.go") || testFile("pkga/a.go") || !testFile("foo_test.py") {
		t.Fatal("test file")
	}
}

func TestContainsIdent(t *testing.T) {
	if !containsIdentifier([]byte("Control PoolKind = iota\n"), "iota") {
		t.Fatal("want iota")
	}
	if containsIdentifier([]byte("iotaFoo"), "iota") {
		t.Fatal("prefix")
	}
}
