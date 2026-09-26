package sitter

// Tree is a parsed file. Root is nil only when Parse failed to produce a tree.
type Tree struct {
	Root     *Node
	Language string
}

// NewNode builds a snapshot node. Engine implementations use this after parse.
func NewNode(typ string, start, end uint32, named bool, kids []Node, fields []string) Node {
	return Node{typ: typ, start: start, end: end, named: named, kids: kids, fields: fields}
}

// NullNode is a missing child slot (IsNull is true).
func NullNode() Node {
	return Node{null: true}
}

// Node is one CST node. Methods match the tree-sitter walk used by tape
// and extract. Child returns a pointer into this tree; keep the Tree reachable.
type Node struct {
	typ    string
	start  uint32
	end    uint32
	named  bool
	null   bool
	kids   []Node
	fields []string
}

// Type is the tree-sitter node type, or "" if n is null.
func (n *Node) Type() string {
	if n == nil || n.null {
		return ""
	}
	return n.typ
}

// StartByte is the inclusive start offset.
func (n *Node) StartByte() uint32 {
	if n == nil || n.null {
		return 0
	}
	return n.start
}

// EndByte is the exclusive end offset.
func (n *Node) EndByte() uint32 {
	if n == nil || n.null {
		return 0
	}
	return n.end
}

// ChildCount is the number of children, including anonymous and null slots.
func (n *Node) ChildCount() uint32 {
	if n == nil || n.null {
		return 0
	}
	return uint32(len(n.kids))
}

// Child returns the child at i, or nil if i is out of range.
func (n *Node) Child(i uint32) *Node {
	if n == nil || n.null || i >= uint32(len(n.kids)) {
		return nil
	}
	return &n.kids[i]
}

// FieldNameForChild is the named field for child i, or "".
func (n *Node) FieldNameForChild(i uint32) string {
	if n == nil || n.null || i >= uint32(len(n.fields)) {
		return ""
	}
	return n.fields[i]
}

// IsNull reports a missing or placeholder node.
func (n *Node) IsNull() bool {
	return n == nil || n.null
}

// IsNamed reports a named (non-anonymous) node.
func (n *Node) IsNamed() bool {
	return n != nil && !n.null && n.named
}

// NamedChildCount is the number of named, non-null children.
func (n *Node) NamedChildCount() uint32 {
	if n == nil || n.null {
		return 0
	}
	var c uint32
	for i := range n.kids {
		if n.kids[i].IsNamed() {
			c++
		}
	}
	return c
}

// NamedChild returns the i-th named child, or nil.
func (n *Node) NamedChild(i uint32) *Node {
	if n == nil || n.null {
		return nil
	}
	var seen uint32
	for j := range n.kids {
		if !n.kids[j].IsNamed() {
			continue
		}
		if seen == i {
			return &n.kids[j]
		}
		seen++
	}
	return nil
}
