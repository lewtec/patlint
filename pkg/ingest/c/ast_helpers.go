package c

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
)

func declaratorIdentity(n *sitter.Node, source []byte) (nameNode *sitter.Node, short, typePrefix string) {
	for n != nil && !n.IsNull() {
		switch n.Type() {
		case "identifier", "field_identifier", "type_identifier":
			return n, ingestutil.NodeText(n, source), typePrefix
		case "destructor_name":
			// Span the full destructor_name (~Type) so entity_text invariants
			// match the symbol leaf; the identifier alone omits '~'.
			if id := ingestutil.ChildByType(n, "identifier"); id != nil {
				return n, "~" + ingestutil.NodeText(id, source), typePrefix
			}
			if id := ingestutil.ChildByType(n, "type_identifier"); id != nil {
				return n, "~" + ingestutil.NodeText(id, source), typePrefix
			}
			return n, ingestutil.NodeText(n, source), typePrefix
		case "operator_name", "operator_cast":
			return n, ingestutil.NodeText(n, source), typePrefix
		case "qualified_identifier":
			if sc := ingestutil.ChildByField(n, "scope"); sc != nil {
				prefix := strings.ReplaceAll(ingestutil.NodeText(sc, source), "::", ".")
				if typePrefix != "" {
					typePrefix = typePrefix + "." + prefix
				} else {
					typePrefix = prefix
				}
			}
			if name := ingestutil.ChildByField(n, "name"); name != nil {
				n = name
				continue
			}
			return nil, "", typePrefix
		case "function_declarator", "pointer_declarator", "array_declarator",
			"parenthesized_declarator", "reference_declarator", "init_declarator",
			"parameter_pack_expansion":
			if d := ingestutil.ChildByField(n, "declarator"); d != nil {
				n = d
				continue
			}
			return nil, "", typePrefix
		default:
			if d := ingestutil.ChildByField(n, "declarator"); d != nil {
				n = d
				continue
			}
			return nil, "", typePrefix
		}
	}
	return nil, "", typePrefix
}

func findFunctionParameters(n *sitter.Node) *sitter.Node {
	for n != nil && !n.IsNull() {
		if n.Type() == "function_declarator" {
			return ingestutil.ChildByField(n, "parameters")
		}
		if d := ingestutil.ChildByField(n, "declarator"); d != nil {
			n = d
			continue
		}
		break
	}
	return nil
}

func isFunctionDeclarator(n *sitter.Node) bool {
	for n != nil && !n.IsNull() {
		if n.Type() == "function_declarator" || n.Type() == "operator_cast" {
			return true
		}
		if d := ingestutil.ChildByField(n, "declarator"); d != nil {
			n = d
			continue
		}
		break
	}
	return false
}

func qualifyName(scope, typePrefix, short string) string {
	if typePrefix != "" {
		short = typePrefix + "." + short
	}
	if scope == "" {
		return short
	}
	if strings.HasPrefix(short, scope+".") {
		return short
	}
	return scope + "." + short
}

func qualifiedLeaf(n *sitter.Node, source []byte) string {
	if n == nil {
		return ""
	}
	for n.Type() == "qualified_identifier" {
		if name := ingestutil.ChildByField(n, "name"); name != nil {
			n = name
			continue
		}
		break
	}
	return ingestutil.NodeText(n, source)
}

func deepestName(n *sitter.Node) *sitter.Node {
	if n == nil {
		return nil
	}
	for n.Type() == "qualified_identifier" {
		if name := ingestutil.ChildByField(n, "name"); name != nil {
			n = name
			continue
		}
		break
	}
	return n
}

// registerParamTypes fills locals map: param name → type leaf (for simple type flow).

func registerParamTypes(params *sitter.Node, source []byte, locals map[string]string) {
	if params == nil || locals == nil {
		return
	}
	for i := uint32(0); i < params.ChildCount(); i++ {
		ch := params.Child(i)
		if ch.Type() != "parameter_declaration" {
			continue
		}
		typ := ingestutil.ChildByField(ch, "type")
		decl := ingestutil.ChildByField(ch, "declarator")
		typeName := simpleTypeLeaf(typ, source)
		if decl != nil {
			// reference_declarator / pointer_declarator wrap the name
			if nameN, short, _ := declaratorIdentity(decl, source); nameN != nil && short != "" && typeName != "" {
				locals[short] = typeName
			}
		}
	}
}

func simpleTypeLeaf(n *sitter.Node, source []byte) string {
	if n == nil {
		return ""
	}
	switch n.Type() {
	case "type_identifier", "identifier":
		return ingestutil.NodeText(n, source)
	case "qualified_identifier":
		return strings.ReplaceAll(ingestutil.NodeText(n, source), "::", ".")
	case "primitive_type", "sized_type_specifier":
		return ""
	case "type_descriptor", "placeholder_type_specifier":
		if t := ingestutil.ChildByField(n, "type"); t != nil {
			return simpleTypeLeaf(t, source)
		}
	}
	// template_type: vector<T> → use template name
	if n.Type() == "template_type" {
		if name := ingestutil.ChildByField(n, "name"); name != nil {
			return simpleTypeLeaf(name, source)
		}
	}
	for i := uint32(0); i < n.ChildCount(); i++ {
		ch := n.Child(i)
		if ch.IsNamed() {
			if s := simpleTypeLeaf(ch, source); s != "" {
				return s
			}
		}
	}
	return ""
}

func registerLocalDeclTypes(decl *sitter.Node, source []byte, locals map[string]string) {
	if decl == nil || locals == nil {
		return
	}
	typ := ingestutil.ChildByField(decl, "type")
	typeName := simpleTypeLeaf(typ, source)
	if typeName == "" {
		return
	}
	if d := ingestutil.ChildByField(decl, "declarator"); d != nil {
		if d.Type() == "init_declarator" {
			d = ingestutil.ChildByField(d, "declarator")
		}
		if nameN, short, _ := declaratorIdentity(d, source); nameN != nil && short != "" && !isFunctionDeclarator(d) {
			locals[short] = typeName
		}
	}
}

func isPrimitiveType(s string) bool {
	switch s {
	case "int", "char", "short", "long", "float", "double", "signed", "unsigned",
		"bool", "void", "size_t", "ssize_t", "ptrdiff_t", "wchar_t", "auto",
		"int8_t", "int16_t", "int32_t", "int64_t",
		"uint8_t", "uint16_t", "uint32_t", "uint64_t",
		"true", "false", "NULL", "nullptr", "this",
		"const", "volatile", "mutable", "constexpr", "consteval", "constinit":
		return true
	default:
		return false
	}
}
