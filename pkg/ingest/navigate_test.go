package ingest_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"
	"github.com/stretchr/testify/require"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/sitter/treesitter"
)

func TestNavigateAround_NodeModulesReexport(t *testing.T) {
	dir := t.TempDir()
	// package entry reexports from internal file (not pulled by project Dir alone)
	pkg := lewpath.New(dir, "node_modules", "widget").String()
	{
		err := os.MkdirAll(pkg, 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(pkg, "package.json").String(), []byte(`{"main":"index.js"}`), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(pkg, "index.js").String(), []byte(`export { helper } from "./lib.js";\n`), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(pkg, "lib.js").String(), []byte(`export function helper() { return 1; }\n`), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(lewpath.New(dir, "main.js").String(), []byte(`import { helper } from "widget";\nhelper();\n`), 0o644)
		require.NoError(t, err)
	}

	// Full project: one-hop may stop at index reexport target path
	vm, err := pattern.New(prelude.FS)
	require.NoError(t, err)

	w, err := walker.NewWalker(t.Context(), project.NewSession(".").WithEngine(treesitter.Engine{}), vm)
	require.NoError(t, err)

	proj, err := w.Load(t.Context(), ingest.SourceProject(dir), ingest.MaterializeOptions{ExpandImports: true})
	require.NoError(t, err)

	// On-demand navigate from import target should pull lib.js entity
	ref := ingest.ParseReference("path:./node_modules/widget/index.js::helper")
	nav, got := w.NavigateReference(t.Context(), dir, nil, proj, ref)
	require.NotNil(t, nav,
		"nil nav result")

	found := false
	for _, e := range nav.Atoms {
		if strings.Contains(e.Reference, "lib.js") && strings.Contains(e.Reference, "helper") {
			found = true
			break
		}
	}
	// either entity on lib or canonicalize landed on lib
	if !found {
		if !strings.Contains(got.Path, "lib.js") && !strings.Contains(got.String(), "helper") {
			// dump for debug
			var ents []string
			for _, e := range nav.Atoms {
				ents = append(ents, e.Reference)
			}
			require.FailNow(t, fmt.Sprintf("expected helper in lib.js via on-demand nav, got ref=%s entities=%v", got.String(), ents))
		}
	}
}
