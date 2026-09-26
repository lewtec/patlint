package ingestutil

import (
	"reflect"
	"testing"
)

func TestForEachStringLiteral(t *testing.T) {
	src := []byte(`x = "hello"; y = ` + "`raw`")
	var got []struct {
		lit   string
		start int
	}
	ForEachStringLiteral(src, func(lit string, start int) bool {
		got = append(got, struct {
			lit   string
			start int
		}{lit, start})
		return true
	})
	want := []struct {
		lit   string
		start int
	}{
		{`"hello"`, 4},
		{"`raw`", 17},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestForEachStringLiteralEscapes(t *testing.T) {
	src := []byte(`"a\"b"`)
	var lits []string
	ForEachStringLiteral(src, func(lit string, start int) bool {
		lits = append(lits, lit)
		return true
	})
	if len(lits) != 1 || lits[0] != `"a\"b"` {
		t.Fatalf("escaped quote: got %#v", lits)
	}
}

func TestForEachStringLiteralSkipsComments(t *testing.T) {
	// Quote inside // comment must not open a false string that swallows the
	// real import path (contapila pdfdslipakv1 extract.go style docs).
	src := []byte(`// show { s }  # Tj / ' / "  (full string)
package p
import "github.com/example/internal/dump"
const c = '"'` + "\n" + `const r = ` + "`raw`" + `
/* "block" */
const d = "after"
`)
	var lits []string
	ForEachStringLiteral(src, func(lit string, start int) bool {
		lits = append(lits, lit)
		return true
	})
	want := []string{`"github.com/example/internal/dump"`, "`raw`", `"after"`}
	if !reflect.DeepEqual(lits, want) {
		t.Fatalf("got %#v want %#v", lits, want)
	}
}
