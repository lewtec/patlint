package ingestutil

import "github.com/lewtec/patlint/pkg/sitter"

// NodeText returns the source text covered by a node.
func NodeText(n *sitter.Node, source []byte) string {
	if n == nil {
		return ""
	}
	s, e := n.StartByte(), n.EndByte()
	if s <= e && int(e) <= len(source) {
		return string(source[s:e])
	}
	return ""
}

// ChildByField returns the first child whose field name matches, or nil.
func ChildByField(n *sitter.Node, field string) *sitter.Node {
	if n == nil {
		return nil
	}
	for i := uint32(0); i < n.ChildCount(); i++ {
		if n.FieldNameForChild(i) == field {
			c := n.Child(i)
			if c != nil && !c.IsNull() {
				return c
			}
		}
	}
	return nil
}

// ChildByType returns the first child whose node type matches, or nil.
func ChildByType(n *sitter.Node, typ string) *sitter.Node {
	if n == nil {
		return nil
	}
	for i := uint32(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c != nil && !c.IsNull() && c.Type() == typ {
			return c
		}
	}
	return nil
}
