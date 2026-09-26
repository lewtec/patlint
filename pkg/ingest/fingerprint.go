package ingest

import (
	"hash/fnv"
	"math/bits"
)

// AbstractFeatures returns a multiset of unigram, bigram, and trigram features
// from an abstract term stream. Used for gradual similarity (not cryptographic
// hashing).
//
// Small edits to terms add/remove a few features; the rest of the multiset stays.
// Trigrams keep order-sensitive differences (e.g. Max vs Min return arms) without
// the avalanche of a full-string hash.
func AbstractFeatures(terms []string) map[string]int {
	if len(terms) == 0 {
		return nil
	}
	f := make(map[string]int, len(terms)*3)
	for i, t := range terms {
		if t == "" {
			continue
		}
		f["u:"+t]++
		if i+1 < len(terms) && terms[i+1] != "" {
			f["b:"+t+"\x00"+terms[i+1]]++
		}
		if i+2 < len(terms) && terms[i+1] != "" && terms[i+2] != "" {
			f["t:"+t+"\x00"+terms[i+1]+"\x00"+terms[i+2]]++
		}
	}
	return f
}

// FeatureJaccard is |A∩B| / |A∪B| over feature *keys* (presence, not counts).
// 1 = same feature set, 0 = disjoint. Stable under small multiset edits.
func FeatureJaccard(a, b map[string]int) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// WeightedFeatureJaccard uses min/max counts (multiset Jaccard).
func WeightedFeatureJaccard(a, b map[string]int) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	var inter, union int
	for k, ca := range a {
		cb := b[k]
		if ca < cb {
			inter += ca
			union += cb
		} else {
			inter += cb
			union += ca
		}
	}
	for k, cb := range b {
		if _, ok := a[k]; !ok {
			union += cb
		}
	}
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// SimHash64 is a 64-bit locality-sensitive fingerprint of abstract terms.
// Hamming distance grows roughly with feature disagreement (Charikar-style):
// small term-stream edits flip few bits; unrelated streams look random (~32 bits).
//
// Prefer FeatureJaccard / WeightedFeatureJaccard for ranking; use SimHash for
// candidate filtering (compare Hamming ≤ k) or compact storage.
func SimHash64(terms []string) uint64 {
	return simHashFromFeatures(AbstractFeatures(terms))
}

func simHashFromFeatures(feats map[string]int) uint64 {
	if len(feats) == 0 {
		return 0
	}
	var acc [64]int
	for feat, w := range feats {
		if w <= 0 {
			continue
		}
		h := hashFeature64(feat)
		for bit := 0; bit < 64; bit++ {
			if h&(uint64(1)<<uint(bit)) != 0 {
				acc[bit] += w
			} else {
				acc[bit] -= w
			}
		}
	}
	var out uint64
	for bit := 0; bit < 64; bit++ {
		if acc[bit] > 0 {
			out |= uint64(1) << uint(bit)
		}
	}
	return out
}

// Hamming64 returns the number of differing bits (0 = identical SimHash).
func Hamming64(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// SimHashSimilarity maps Hamming distance to [0,1] (1 = identical).
func SimHashSimilarity(a, b uint64) float64 {
	return 1 - float64(Hamming64(a, b))/64
}

func hashFeature64(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// AbstractFingerprint is a gradual fingerprint for one abstract unit.
type AbstractFingerprint struct {
	// SimHash is LSH over unigram+bigram features (compact, approximate).
	SimHash uint64
	// Features is the full multiset (authoritative for ranking).
	Features map[string]int
}

// FingerprintTerms builds a gradual fingerprint from abstract terms.
func FingerprintTerms(terms []string) AbstractFingerprint {
	feats := AbstractFeatures(terms)
	return AbstractFingerprint{
		SimHash:  simHashFromFeatures(feats),
		Features: feats,
	}
}

// Similarity scores two fingerprints in [0,1] using weighted feature Jaccard
// (primary) blended lightly with SimHash similarity so tiny LSH noise does not
// dominate. Prefer this over equality of a cryptographic hash.
func (f AbstractFingerprint) Similarity(other AbstractFingerprint) float64 {
	j := WeightedFeatureJaccard(f.Features, other.Features)
	// If one side has no features, fall back to SimHash only.
	if len(f.Features) == 0 || len(other.Features) == 0 {
		return SimHashSimilarity(f.SimHash, other.SimHash)
	}
	// Jaccard is the continuous measure; SimHash is a weak regularizer.
	s := SimHashSimilarity(f.SimHash, other.SimHash)
	return 0.85*j + 0.15*s
}
