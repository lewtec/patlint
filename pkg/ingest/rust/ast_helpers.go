package rust

import (
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
)

func qualify(scope, name string) string {
	if scope == "" {
		return name
	}
	return scope + "." + name
}

func appendAtom(fe *project.FileExtract, nameNode *sitter.Node, source []byte, name string, exported bool) {
	if nameNode == nil || name == "" {
		return
	}
	fe.Atoms = append(fe.Atoms, project.AtomDef{
		Name:      name,
		StartByte: nameNode.StartByte(),
		EndByte:   nameNode.EndByte(),
		Exported:  exported,
	})
}

func rustTypeName(n *sitter.Node, source []byte) string {
	if n == nil {
		return ""
	}
	switch n.Type() {
	case "type_identifier", "identifier":
		return ingestutil.NodeText(n, source)
	case "generic_type":
		if t := ingestutil.ChildByField(n, "type"); t != nil {
			return rustTypeName(t, source)
		}
		for i := uint32(0); i < n.ChildCount(); i++ {
			ch := n.Child(i)
			if ch.Type() == "type_identifier" {
				return ingestutil.NodeText(ch, source)
			}
		}
	case "scoped_type_identifier":
		if name := ingestutil.ChildByField(n, "name"); name != nil {
			return ingestutil.NodeText(name, source)
		}
	}
	// fallback: first type_identifier descendant
	var found string
	var walk func(*sitter.Node)
	walk = func(x *sitter.Node) {
		if x == nil || found != "" {
			return
		}
		if x.Type() == "type_identifier" {
			found = ingestutil.NodeText(x, source)
			return
		}
		for i := uint32(0); i < x.ChildCount(); i++ {
			walk(x.Child(i))
		}
	}
	walk(n)
	return found
}

func joinRustPath(prefix, leaf string) string {
	leaf = trimRustPath(leaf)
	if prefix == "" {
		return leaf
	}
	if leaf == "" {
		return prefix
	}
	return prefix + "::" + leaf
}

func trimRustPath(s string) string {
	return s
}

func splitRustPath(p string) []string {
	if p == "" {
		return nil
	}
	return splitSeq(p, "::")
}

func joinSegs(segs []string) string {
	if len(segs) == 0 {
		return ""
	}
	out := segs[0]
	for i := 1; i < len(segs); i++ {
		out += "::" + segs[i]
	}
	return out
}

func leafOfPath(p string) string {
	segs := splitRustPath(p)
	if len(segs) == 0 {
		return p
	}
	return segs[len(segs)-1]
}

func splitSeq(s, sep string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for {
		i := indexStr(s, sep)
		if i < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:i])
		s = s[i+len(sep):]
	}
}

func indexStr(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}
