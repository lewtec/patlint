package js

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
)

type jsImportStmt struct {
	// Full text of the import statement including trailing semicolon/newline.
	text string
	// Local names this import introduces (default, named, and namespace).
	locals []string
	// namedLocals are only from { named } specs (for prune).
	namedLocals []string
	// namedOnly is true when the clause is only { named } (no default/namespace/side-effect).
	namedOnly bool
	// startByte/endByte in the source file.
	startByte, endByte uint32
}

// parseJSImportStatements extracts import_statement nodes from the tree-sitter root.
func parseJSImportStatements(root *sitter.Node, source []byte) []jsImportStmt {
	var stmts []jsImportStmt
	for i := uint32(0); i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child.Type() != "import_statement" {
			continue
		}
		text := ingestutil.NodeText(child, source)
		stmt := jsImportStmt{
			text:      text,
			startByte: child.StartByte(),
			endByte:   child.EndByte(),
		}
		clause := ingestutil.ChildByType(child, "import_clause")
		if clause == nil {
			stmt.namedOnly = false
		} else {
			var hasDefault, hasNamespace, hasNamed bool
			collectImportLocals(clause, source, &stmt.locals, &stmt.namedLocals, &hasDefault, &hasNamespace, &hasNamed)
			stmt.namedOnly = hasNamed && !hasDefault && !hasNamespace
		}
		stmts = append(stmts, stmt)
	}
	return stmts
}

func jsImportModuleSpec(stmt string) (mod string, quote byte) {
	// Prefer the last quoted string (handles import("x") rarely; normal is from 'x').
	for _, q := range []byte{'\'', '"'} {
		// Find from '…' / from "…"
		key := " from "
		idx := strings.LastIndex(stmt, key)
		if idx < 0 {
			// side-effect: import 'mod'
			idx = strings.Index(stmt, "import ")
			if idx < 0 {
				continue
			}
			rest := stmt[idx+len("import "):]
			rest = strings.TrimSpace(rest)
			if len(rest) > 0 && rest[0] == q {
				end := strings.IndexByte(rest[1:], q)
				if end >= 0 {
					return rest[1 : 1+end], q
				}
			}
			continue
		}
		rest := strings.TrimSpace(stmt[idx+len(key):])
		if len(rest) == 0 || rest[0] != q {
			continue
		}
		end := strings.IndexByte(rest[1:], q)
		if end < 0 {
			continue
		}
		return rest[1 : 1+end], q
	}
	return "", '\''
}

func collectImportLocals(n *sitter.Node, source []byte, all, named *[]string, hasDefault, hasNamespace, hasNamed *bool) {
	for i := uint32(0); i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch c.Type() {
		case "identifier":
			*hasDefault = true
			*all = append(*all, ingestutil.NodeText(c, source))
		case "named_imports":
			*hasNamed = true
			for j := uint32(0); j < c.ChildCount(); j++ {
				spec := c.Child(j)
				if spec.Type() != "import_specifier" {
					continue
				}
				var local string
				if alias := ingestutil.ChildByField(spec, "alias"); alias != nil {
					local = ingestutil.NodeText(alias, source)
				} else if name := ingestutil.ChildByField(spec, "name"); name != nil {
					local = ingestutil.NodeText(name, source)
				}
				if local == "" {
					continue
				}
				*all = append(*all, local)
				*named = append(*named, local)
			}
		case "namespace_import":
			*hasNamespace = true
			for j := uint32(0); j < c.ChildCount(); j++ {
				id := c.Child(j)
				if id.Type() == "identifier" {
					*all = append(*all, ingestutil.NodeText(id, source))
				}
			}
		}
	}
}
