package pattern

import (
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/stretchr/testify/require"

	_ "github.com/lewtec/patlint/pkg/ingest/go"
)

func TestMatchFileMultiLeaves_EqualsSolo(t *testing.T) {
	dir := t.TempDir()
	src := []byte("package p\nfunc f(x interface{}) int { return 1 }\n")
	path := lewpath.New(dir, "x.go").String()
	mustWrite(t, path, src)
	rootNode := mustParseRoot(t, path)

	m1, err := ParseMatcher(`(token "interface{}")`)
	require.NoError(t, err)

	m2, err := ParseMatcher(`(token "return")`)
	require.NoError(t, err)

	cm1, err := CompileMatcher(m1)
	require.NoError(t, err)

	cm2, err := CompileMatcher(m2)
	require.NoError(t, err)

	a1, fk1, ok := cm1.AsFullFileLeafArm(10)
	require.False(t, !ok || fk1 != fileKeyAlways,
		"arm1 ok=%v key=%q", ok, fk1)

	a2, fk2, ok := cm2.AsFullFileLeafArm(20)
	require.False(t, !ok || fk2 != fileKeyAlways,
		"arm2 ok=%v key=%q", ok, fk2)

	solo1, err := MatchFileMatcher(t.Context(), testSess(), dir, "x.go", src, rootNode, cm1, nil)
	require.NoError(t, err)

	solo2, err := MatchFileMatcher(t.Context(), testSess(), dir, "x.go", src, rootNode, cm2, nil)
	require.NoError(t, err)

	collapsed, err := CompileMultiLeafNFA([]LeafArm{a1, a2})
	require.NoError(t, err)
	require.Greater(t, len(collapsed.n.states), 2,
		"want merged states, got %d", len(collapsed.n.states))

	multi, err := MatchFileMultiLeavesNFA(testSess(), dir, "x.go", src, rootNode, collapsed, []LeafArm{a1, a2}, nil)
	require.NoError(t, err)

	byID := map[int][]Match{}
	for _, tm := range multi {
		byID[tm.ID] = append(byID[tm.ID], tm.Match)
	}
	assertSameMatches(t, "id10", solo1, byID[10])
	assertSameMatches(t, "id20", solo2, byID[20])
}

func assertSameMatches(t *testing.T, label string, want, got []Match) {
	t.Helper()
	require.Len(t, want, len(got),
		"%s: len want %d got %d", label, len(want), len(got))

	for i := range want {
		require.False(t, want[i].StartByte != got[i].StartByte || want[i].EndByte != got[i].EndByte,
			"%s[%d]: want %d-%d got %d-%d", label, i,
			want[i].StartByte, want[i].EndByte, got[i].StartByte, got[i].EndByte)

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
	require.NoError(t, err)

	m2, err := ParseMatcher(`(token "func")`)
	require.NoError(t, err)

	cm1, err := CompileMatcher(m1)
	require.NoError(t, err)

	cm2, err := CompileMatcher(m2)
	require.NoError(t, err)

	a1, _, _ := cm1.AsFullFileLeafArm(1)
	a2, _, _ := cm2.AsFullFileLeafArm(2)
	solo1, err := MatchFileMatcher(t.Context(), testSess(), dir, "x.go", src, rootNode, cm1, nil)
	require.NoError(t, err)

	multi, err := MatchFileMultiLeaves(testSess(), dir, "x.go", src, rootNode, []LeafArm{a1, a2}, nil)
	require.NoError(t, err)

	var got1 []Match
	for _, tm := range multi {
		if tm.ID == 1 {
			got1 = append(got1, tm.Match)
		}
	}
	assertSameMatches(t, "cap", solo1, got1)
	require.False(t, len(got1) > 0 && len(got1[0].Captures["n"]) == 0,
		"want capture n on multi hit: %+v", got1[0].Captures)

}

func TestAsFullFileLeafArm_RejectsUnder(t *testing.T) {
	m, err := ParseMatcher(`(under (token "func") (token "x"))`)
	require.NoError(t, err)

	cm, err := CompileMatcher(m)
	require.NoError(t, err)

	_, _, ok := cm.AsFullFileLeafArm(0)
	require.False(t, ok,
		"under must not fuse as full-file leaf")

}
