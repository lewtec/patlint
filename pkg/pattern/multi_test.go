package pattern

import (
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestMatchFileMultiLeaves_EqualsSolo(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) int { return 1 }\n")
	path := lewpath.New(dir, "x.go").String()
	mustWrite(t, path, src)
	rootNode := mustParseRoot(t, path)

	m1, err := ParseMatcher(`(token "interface{}")`)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := ParseMatcher(`(token "return")`)
	if err != nil {
		t.Fatal(err)
	}
	cm1, err := CompileMatcher(m1)
	if err != nil {
		t.Fatal(err)
	}
	cm2, err := CompileMatcher(m2)
	if err != nil {
		t.Fatal(err)
	}
	a1, fk1, ok := cm1.AsFullFileLeafArm(10)
	if !ok || fk1 != fileKeyAlways {
		t.Fatalf("arm1 ok=%v key=%q", ok, fk1)
	}
	a2, fk2, ok := cm2.AsFullFileLeafArm(20)
	if !ok || fk2 != fileKeyAlways {
		t.Fatalf("arm2 ok=%v key=%q", ok, fk2)
	}

	solo1, err := MatchFileMatcher(testSess(), dir, "x.go", src, rootNode, cm1, nil)
	if err != nil {
		t.Fatal(err)
	}
	solo2, err := MatchFileMatcher(testSess(), dir, "x.go", src, rootNode, cm2, nil)
	if err != nil {
		t.Fatal(err)
	}
	collapsed, err := CompileMultiLeafNFA([]LeafArm{a1, a2})
	if err != nil {
		t.Fatal(err)
	}
	if len(collapsed.n.states) <= 2 {
		t.Fatalf("want merged states, got %d", len(collapsed.n.states))
	}
	multi, err := MatchFileMultiLeavesNFA(testSess(), dir, "x.go", src, rootNode, collapsed, []LeafArm{a1, a2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int][]Match{}
	for _, tm := range multi {
		byID[tm.ID] = append(byID[tm.ID], tm.Match)
	}
	assertSameMatches(t, "id10", solo1, byID[10])
	assertSameMatches(t, "id20", solo2, byID[20])
}

func assertSameMatches(t *testing.T, label string, want, got []Match) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: len want %d got %d", label, len(want), len(got))
	}
	for i := range want {
		if want[i].StartByte != got[i].StartByte || want[i].EndByte != got[i].EndByte {
			t.Fatalf("%s[%d]: want %d-%d got %d-%d", label, i,
				want[i].StartByte, want[i].EndByte, got[i].StartByte, got[i].EndByte)
		}
	}
}

func TestCollapsedMulti_CaptureArm(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\nfunc f() { x := 1; y := 2 }\n")
	path := lewpath.New(dir, "x.go").String()
	mustWrite(t, path, src)
	rootNode := mustParseRoot(t, path)

	// Two capturing leaves
	m1, err := ParseMatcher(`(seq (capture n (regex "^[a-z]")) (token ":="))`)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := ParseMatcher(`(token "func")`)
	if err != nil {
		t.Fatal(err)
	}
	cm1, err := CompileMatcher(m1)
	if err != nil {
		t.Fatal(err)
	}
	cm2, err := CompileMatcher(m2)
	if err != nil {
		t.Fatal(err)
	}
	a1, _, _ := cm1.AsFullFileLeafArm(1)
	a2, _, _ := cm2.AsFullFileLeafArm(2)
	solo1, err := MatchFileMatcher(testSess(), dir, "x.go", src, rootNode, cm1, nil)
	if err != nil {
		t.Fatal(err)
	}
	multi, err := MatchFileMultiLeaves(testSess(), dir, "x.go", src, rootNode, []LeafArm{a1, a2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got1 []Match
	for _, tm := range multi {
		if tm.ID == 1 {
			got1 = append(got1, tm.Match)
		}
	}
	assertSameMatches(t, "cap", solo1, got1)
	if len(got1) > 0 && len(got1[0].Captures["n"]) == 0 {
		t.Fatalf("want capture n on multi hit: %+v", got1[0].Captures)
	}
}

func TestAsFullFileLeafArm_RejectsUnder(t *testing.T) {
	m, err := ParseMatcher(`(under (token "func") (token "x"))`)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := CompileMatcher(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := cm.AsFullFileLeafArm(0); ok {
		t.Fatal("under must not fuse as full-file leaf")
	}
}
