package zig

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
)

func registerParamTypes(params *sitter.Node, source []byte, types map[string]string) {
	if params == nil || types == nil {
		return
	}
	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil {
			return
		}
		if n.Type() == "parameter" {
			pn := ingestutil.ChildByField(n, "name")
			if pn == nil {
				pn = firstIdentifier(n)
			}
			if pn == nil {
				return
			}
			name := ingestutil.NodeText(pn, source)
			if name == "" || name == "_" {
				return
			}
			if typ := ingestutil.ChildByField(n, "type"); typ != nil {
				if t := zigExprTypeName(typ, source); t != "" {
					types[name] = t
				}
			}
			return
		}
		for i := uint32(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(params)
}

// zigExprTypeName is the type ident of a constructor/call/ident expression
// (color.CIELCHuvAlpha.fromLuvAlpha → CIELCHuvAlpha; self: CIELCHuvAlpha → CIELCHuvAlpha).
// Pointer/optional/error wrappers unwrap (*Node, *const Node, ?T, E!T) so
// self.field on a pointer receiver still binds to Node.field.

func zigExprTypeName(n *sitter.Node, source []byte) string {
	if n == nil || n.IsNull() {
		return ""
	}
	switch n.Type() {
	case "identifier":
		return ingestutil.NodeText(n, source)
	case "call_expression":
		for i := uint32(0); i < n.ChildCount(); i++ {
			ch := n.Child(i)
			switch ch.Type() {
			case "field_expression":
				return zigCallCalleeType(ch, source)
			case "identifier":
				return ingestutil.NodeText(ch, source)
			}
		}
	case "struct_initializer":
		for i := uint32(0); i < n.ChildCount(); i++ {
			ch := n.Child(i)
			if ch.Type() == "field_expression" || ch.Type() == "identifier" {
				return zigLastIdent(ch, source)
			}
		}
	case "field_expression":
		return zigLastIdent(n, source)
	default:
		// *T / ?T / E!T / []T and grammar-specific wrappers.
		for i := uint32(0); i < n.ChildCount(); i++ {
			ch := n.Child(i)
			if ch == nil || ch.IsNull() || !ch.IsNamed() {
				continue
			}
			if t := zigExprTypeName(ch, source); t != "" {
				return t
			}
		}
	}
	return ""
}

func zigLastIdent(n *sitter.Node, source []byte) string {
	if n == nil {
		return ""
	}
	if n.Type() == "identifier" {
		return ingestutil.NodeText(n, source)
	}
	if n.Type() == "field_expression" {
		if m := ingestutil.ChildByField(n, "member"); m != nil {
			return ingestutil.NodeText(m, source)
		}
	}
	return ""
}

// zigCallCalleeType: Type.method → Type; module.Type.method → Type.

func zigCallCalleeType(n *sitter.Node, source []byte) string {
	if n == nil || n.Type() != "field_expression" {
		return ""
	}
	obj := ingestutil.ChildByField(n, "object")
	if obj == nil {
		return ""
	}
	if obj.Type() == "identifier" {
		return ingestutil.NodeText(obj, source)
	}
	if obj.Type() == "field_expression" {
		if m := ingestutil.ChildByField(obj, "member"); m != nil {
			return ingestutil.NodeText(m, source)
		}
	}
	return ""
}

// zigWalkIfWhile walks if/while, binding |payload| to the unwrapped
// condition type so rle.deinit() follows TargaRLEDecoder.deinit.

func zigPayloadIdents(payload *sitter.Node, source []byte) []string {
	if payload == nil {
		return nil
	}
	var names []string
	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil || n.IsNull() {
			return
		}
		if n.Type() == "identifier" {
			nm := ingestutil.NodeText(n, source)
			if nm != "" && nm != "_" {
				names = append(names, nm)
			}
			return
		}
		for i := uint32(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(payload)
	return names
}

func registerParamNames(params *sitter.Node, source []byte, locals map[string]struct{}) {
	if params == nil || locals == nil {
		return
	}
	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil {
			return
		}
		if n.Type() == "parameter" {
			if pn := ingestutil.ChildByField(n, "name"); pn != nil {
				name := ingestutil.NodeText(pn, source)
				if name != "" && name != "_" {
					locals[name] = struct{}{}
				}
			}
			return
		}
		for i := uint32(0); i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(params)
}

func qualify(scope, name string) string {
	if scope == "" {
		return name
	}
	return scope + "." + name
}

// zigFieldChain flattens a nested field_expression into import/root ident +
// dotted remainder. utils.Markers.app14 → ("utils", "Markers.app14", …).

func zigFieldChain(obj, member *sitter.Node, source []byte) (root, rest string, rootStart, rootEnd uint32, ok bool) {
	if obj == nil || member == nil {
		return "", "", 0, 0, false
	}
	parts := []string{ingestutil.NodeText(member, source)}
	n := obj
	for n != nil && n.Type() == "field_expression" {
		m := ingestutil.ChildByField(n, "member")
		o := ingestutil.ChildByField(n, "object")
		if m == nil || ingestutil.NodeText(m, source) == "" {
			return "", "", 0, 0, false
		}
		parts = append([]string{ingestutil.NodeText(m, source)}, parts...)
		n = o
	}
	if n == nil || n.Type() != "identifier" {
		return "", "", 0, 0, false
	}
	root = ingestutil.NodeText(n, source)
	if root == "" || parts[0] == "" {
		return "", "", 0, 0, false
	}
	return root, strings.Join(parts, "."), n.StartByte(), n.EndByte(), true
}

func isPub(n *sitter.Node) bool {
	if n == nil {
		return false
	}
	for i := uint32(0); i < n.ChildCount(); i++ {
		if n.Child(i).Type() == "pub" {
			return true
		}
	}
	return false
}

func firstIdentifier(n *sitter.Node) *sitter.Node {
	if n == nil {
		return nil
	}
	for i := uint32(0); i < n.ChildCount(); i++ {
		ch := n.Child(i)
		if ch.Type() == "identifier" {
			return ch
		}
	}
	return nil
}
