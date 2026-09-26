package ingest

import "testing"

func TestQualifierBefore(t *testing.T) {
	src := []byte("pkga.Helper()\nmedia.PlaybackStatus\nHelper()\n")
	if qs, qe, ok := qualifierBefore(src, uint32(5)); !ok || string(src[qs:qe]) != "pkga" {
		t.Fatalf("pkga: ok=%v %q", ok, src[qs:qe])
	}
	at := uint32(len("pkga.Helper()\nmedia."))
	if qs, qe, ok := qualifierBefore(src, at); !ok || string(src[qs:qe]) != "media" {
		t.Fatalf("media: ok=%v %q", ok, src[qs:qe])
	}
	bare := uint32(len("pkga.Helper()\nmedia.PlaybackStatus\n"))
	if _, _, ok := qualifierBefore(src, bare); ok {
		t.Fatal("bare Helper has no qualifier")
	}
	if qs, qe, ok := qualifierInSpan([]byte("pkga.Helper")); !ok || string([]byte("pkga.Helper")[qs:qe]) != "pkga" {
		t.Fatalf("span: ok=%v", ok)
	}
}
