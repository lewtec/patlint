// Package treesitter is the [sitter.Engine] backed by the lewkit tree-sitter driver.
// Grammars still register through the modules imported below.
package treesitter

import (
	"context"
	"errors"
	"fmt"

	lewts "github.com/lewtec/lewkit/x/driver/treesitter"
	"github.com/lewtec/patlint/pkg/sitter"

	_ "github.com/lewtec/lewkit/x/driver/treesitter/prelude"

	// Product grammars. Keep in sync with internal/prelude language_*.rft.
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/astro"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/c"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/commonlisp"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/cpp"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/go"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/html"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/java"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/javascript"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/kotlin"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/nix"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/python"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/rust"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/scala"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/svelte"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/templ"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/tsx"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/typescript"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/vue"
	_ "github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar/zig"
)

// Engine parses through the lewkit tree-sitter driver.
// Languages register via the blank imports.
type Engine struct{}

var _ sitter.Engine = Engine{}

// Has implements [sitter.Engine].
func (Engine) Has(ctx context.Context, language string) bool {
	if ctx == nil || language == "" {
		return false
	}
	_, err := lewts.Get(ctx, language)
	return err == nil
}

// Parse implements [sitter.Engine].
func (Engine) Parse(ctx context.Context, src []byte, language string) (*sitter.Tree, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := sitter.CheckLanguage(language); err != nil {
		return nil, err
	}
	gt, err := lewts.Parse(ctx, language, src)
	if err != nil {
		if errors.Is(err, lewts.ErrUnknown) {
			return nil, fmt.Errorf("%w: %q", sitter.ErrUnsupportedLanguage, language)
		}
		return nil, fmt.Errorf("%w: %q: %w", sitter.ErrSetLanguage, language, err)
	}
	root := snapshot(gt.RootNode())
	if root.IsNull() {
		return &sitter.Tree{Language: language}, nil
	}
	return &sitter.Tree{Root: &root, Language: language}, nil
}

func snapshot(n lewts.Node) sitter.Node {
	if n == nil || n.IsNull() {
		return sitter.NullNode()
	}
	cc := n.ChildCount()
	if cc == 0 {
		return sitter.NewNode(n.Type(), n.StartByte(), n.EndByte(), n.IsNamed(), nil, nil)
	}
	kids := make([]sitter.Node, cc)
	fields := make([]string, cc)
	for i := range cc {
		fields[i] = n.FieldNameForChild(i)
		kids[i] = snapshot(n.Child(i))
	}
	return sitter.NewNode(n.Type(), n.StartByte(), n.EndByte(), n.IsNamed(), kids, fields)
}
