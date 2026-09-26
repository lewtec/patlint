package ingest_test

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestFingerprint_GradualVsAvalanche(t *testing.T) {
	// Same abstract shape, one hole reuse change (Max vs Min style).
	maxT := []string{"{", "if", "%r1", ">", "%r2", "{", "return", "%r1", "}", "return", "%r2", "}"}
	minT := []string{"{", "if", "%r1", ">", "%r2", "{", "return", "%r2", "}", "return", "%r1", "}"}
	// Unrelated
	other := []string{"for", "%r1", ":=", "range", "%r2", "{", "%r1", "++", "}"}

	fm := ingest.FingerprintTerms(maxT)
	fn := ingest.FingerprintTerms(minT)
	fo := ingest.FingerprintTerms(other)

	hamMN := ingest.Hamming64(fm.SimHash, fn.SimHash)
	hamMO := ingest.Hamming64(fm.SimHash, fo.SimHash)
	require.Less(t, hamMN, hamMO,
		"Max~Min hamming %d should be < Max~other %d", hamMN, hamMO)

	simMN := fm.Similarity(fn)
	simMO := fm.Similarity(fo)
	require.Greater(t, simMN, simMO,
		"Max~Min sim %v should be > Max~other %v", simMN, simMO)
	// Max vs Min share structure but differ in return-arm order (trigrams).
	require.Less(t, simMN, 0.999, "Max and Min should not be identical fingerprints")
	// Identical → perfect
	require.GreaterOrEqual(t, fm.Similarity(fm), 0.999)

	// One-term edit should not go to ~0 similarity
	edited := append([]string(nil), maxT...)
	edited[len(edited)-1] = "return" // small change
	fe := ingest.FingerprintTerms(edited)
	require.GreaterOrEqual(t, fm.Similarity(fe), 0.5,
		"one-term edit sim too low: %v hamming=%d", fm.Similarity(fe), ingest.Hamming64(fm.SimHash, fe.SimHash))

	t.Logf("Max~Min jaccard/sim=%v ham=%d; Max~other sim=%v ham=%d", simMN, hamMN, simMO, hamMO)
}

func TestFingerprint_MaxMinFromSource(t *testing.T) {
	src := `package p
func Max(a, b int) int {
	if a > b { return a }
	return b
}
func Min(a, b int) int {
	if a > b { return b }
	return a
}
func Loop(xs []int) {
	for _, x := range xs { _ = x }
}
`
	root, source, cleanup := parseGo(t, src)
	defer cleanup()
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), vm)
	require.NoError(t, err)

	units, err := w.AbstractFile(t.Context(), root, source, "x.go", true)
	require.NoError(t, err)

	by := map[string]ingest.AbstractFingerprint{}
	for _, u := range units {
		by[u.Name] = ingest.FingerprintTerms(u.Terms)
	}
	require.Greater(t, by["Max"].Similarity(by["Min"]), by["Max"].Similarity(by["Loop"]),
		"Max should be closer to Min than Loop: Max~Min=%v Max~Loop=%v",
		by["Max"].Similarity(by["Min"]), by["Max"].Similarity(by["Loop"]))

}
