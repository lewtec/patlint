package pattern_test

import (
	"fmt"
	"os"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestUseScope_MultiLang(t *testing.T) {
	cases := []struct {
		path, lang string
		src        []byte
		useName    string
		wantScope  string
	}{
		{"a.py", "python", []byte("def main():\n    helper()\n"), "helper", "main"},
		{"a.js", "javascript", []byte("function main() {\n  helper();\n}\n"), "helper", "main"},
		{"a.ts", "typescript", []byte("function main() {\n  helper();\n}\n"), "helper", "main"},
		{"A.java", "java", []byte("class A {\n  void main() { helper(); }\n}\n"), "helper", "main"},
		{"a.rs", "rust", []byte("fn main() {\n    helper();\n}\n"), "helper", "main"},
		{"a.c", "c", []byte("void main() {\n  helper();\n}\n"), "helper", "main"},
		{"a.kt", "kotlin", []byte("fun main() {\n  helper()\n}\n"), "helper", "main"},
		{"a.scala", "scala", []byte("def main() = {\n  helper()\n}\n"), "helper", "main"},
		{"a.zig", "zig", []byte("fn main() void {\n    helper();\n}\n"), "helper", "main"},
	}
	vm, err := pattern.New(os.DirFS(lewpath.New("..", "..", "internal", "prelude").String()))
	require.NoError(t, err)

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			pf, err := ingestutil.ParseSource(t.Context(), ccgo.Engine{}, tc.src, tc.path, tc.lang)
			require.NoError(t, err)

			defer pf.Close()
			fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "", pf.Root, tc.src, tc.path)
			require.NoError(t, err)

			var got string
			for _, u := range fe.Usages {
				if u.Name == tc.useName {
					got = u.Scope
					if got == tc.wantScope {
						return
					}
				}
			}
			require.FailNow(t, fmt.Sprintf("Scope for %q = %q want %q; usages=%+v", tc.useName, got, tc.wantScope, fe.Usages))
		})
	}
}
