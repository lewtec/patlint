// Package ccgo is the ccgo-tree-sitter [sitter.Engine].
package ccgo

import (
	"fmt"

	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/modernc-tree-sitter/ccgo-tree-sitter/grammar"

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

// Engine is the ccgo-tree-sitter backend. Languages register via the blank imports.
type Engine struct{}

var _ sitter.Engine = Engine{}

// Has implements [sitter.Engine].
func (Engine) Has(language string) bool {
	if language == "" {
		return false
	}
	_, ok := grammar.Get(language)
	return ok
}

// Parse implements [sitter.Engine].
func (Engine) Parse(src []byte, language string) (*sitter.Tree, error) {
	if err := sitter.CheckLanguage(language); err != nil {
		return nil, err
	}
	lang, ok := grammar.Get(language)
	if !ok {
		return nil, fmt.Errorf("%w: %q", sitter.ErrUnsupportedLanguage, language)
	}
	p := grammar.NewParser()
	if !p.SetLanguage(lang) {
		p.Delete()
		return nil, fmt.Errorf("%w: %q", sitter.ErrSetLanguage, language)
	}
	gt := p.ParseBytes(src)
	root := snapshot(gt.RootNode())
	gt.Delete()
	p.Delete()
	if root.IsNull() {
		return &sitter.Tree{Language: language}, nil
	}
	return &sitter.Tree{Root: &root, Language: language}, nil
}

func snapshot(n *grammar.Node) sitter.Node {
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
