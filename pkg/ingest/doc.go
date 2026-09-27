package ingest

import (
	"context"
	"fmt"
	"github.com/lewtec/patlint/pkg/projectfs"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

// Pack (as-docstring KIND) values. Mechanism in extractDocFromAST.
const (
	DocstringCommentBefore  = "comment-before"
	DocstringCommentBeforeC = "comment-before-c"
	DocstringBodyString     = "body-string"
)

// DocResult holds documentation extracted for a symbol.
type DocResult struct {
	Name      string
	Signature string
	DocString string
}

// DocFromResult finds the atom for ref in an already-loaded Result and extracts docs.
func DocFromResult(ctx context.Context, policy PackQueries, dir string, result *project.Result, ref Reference) (*DocResult, error) {
	reference := ref.String()
	var entity *project.Atom
	for i := range result.Atoms {
		if result.Atoms[i].Reference == reference {
			entity = &result.Atoms[i]
			break
		}
	}
	if entity == nil && ref.Name != "" {
		for i := range result.Atoms {
			er := ParseReference(result.Atoms[i].Reference)
			if er.Name == ref.Name && SameScopePath(ref, er) {
				entity = &result.Atoms[i]
				ref = er
				break
			}
		}
	}
	if entity == nil {
		return nil, fmt.Errorf("%w: %s", ErrEntityNotFound, reference)
	}
	return docForEntity(ctx, policy, dir, result, ref, entity)
}

// DocFromProviderResult picks the unique provider symbol in an already-loaded Result.
func DocFromProviderResult(ctx context.Context, policy PackQueries, result *project.Result, ref Reference, target ProviderSymbolTarget) (*DocResult, error) {
	if result == nil {
		return nil, fmt.Errorf("%w: %s", ErrEntityNotFound, ref.String())
	}

	langOf := map[string]string{}
	for _, f := range result.Files {
		langOf[f.Path] = f.Language
	}

	symbol := target.Name
	if symbol == "" {
		symbol = ref.Name
	}

	matches := make([]int, 0, 1)
	for i := range result.Atoms {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entityRef := ParseReference(result.Atoms[i].Reference)
		entPath := strings.TrimPrefix(entityRef.Path, "./")
		if !providerAllowDocEntity(ctx, ref, entityRef, entPath, langOf[entPath], result.Families) {
			continue
		}
		if entityRef.Name == symbol {
			matches = append(matches, i)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrEntityNotFound, ref.String())
	}
	if len(matches) > 1 {
		candidates := make([]string, 0, len(matches))
		for _, idx := range matches {
			candidates = append(candidates, result.Atoms[idx].Reference)
		}
		return nil, fmt.Errorf("%w %q in %q package %q, matches: %s", ErrAmbiguousSymbol, symbol, ref.Provider, ref.Path, strings.Join(candidates, ", "))
	}

	entity := &result.Atoms[matches[0]]
	entityRef := ParseReference(entity.Reference)
	return docForEntity(ctx, policy, target.Dir, result, entityRef, entity)
}

func docForEntity(ctx context.Context, policy PackQueries, dir string, result *project.Result, ref Reference, entity *project.Atom) (*DocResult, error) {
	relPath := strings.TrimPrefix(ref.Path, "./")
	filePath := lewpath.New(dir, relPath).String()

	var language string
	for _, f := range result.Files {
		if f.Path == relPath {
			language = f.Language
			break
		}
	}

	if language == "" {
		return nil, fmt.Errorf("%w for %s", ErrUnsupportedLanguage, filePath)
	}
	if err := ingestutil.RequireGrammar(ctx, ccgo.Engine{}, language); err != nil {
		return nil, fmt.Errorf("%w for %s: %v", ErrUnsupportedLanguage, filePath, err)
	}

	source, err := (projectfs.OS{}).ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	pf, err := ingestutil.ParseSource(ctx, ccgo.Engine{}, source, filePath, language)
	if err != nil {
		return nil, err
	}
	defer pf.Close()

	return extractDocFromAST(policy, pf.Root, pf.Source, entity.StartByte, ref.Name, language)
}

func extractDocFromAST(policy PackQueries, root *sitter.Node, source []byte, nameStart uint32, name, language string) (*DocResult, error) {
	declNode := findDeclContaining(policy, language, root, nameStart)
	if declNode == nil {
		return &DocResult{Name: name}, nil
	}

	sig := extractSignature(declNode, source)
	return &DocResult{
		Name:      name,
		Signature: sig,
		DocString: extractDocstring(policy, language, declNode, source),
	}, nil
}

func extractDocstring(policy PackQueries, lang string, decl *sitter.Node, source []byte) string {
	if policy == nil || lang == "" || decl == nil {
		return ""
	}
	switch policy.DocstringKind(lang) {
	case DocstringCommentBefore:
		return ExtractCommentBefore(decl, source, ingestutil.IsSlashSlashCommentLine)
	case DocstringCommentBeforeC:
		return ExtractCommentBefore(decl, source, ingestutil.IsCStyleDocCommentLine)
	case DocstringBodyString:
		return extractBodyStringDoc(decl, source)
	default:
		return ""
	}
}

func extractBodyStringDoc(funcNode *sitter.Node, source []byte) string {
	body := ingestutil.ChildByField(funcNode, "body")
	if body == nil {
		return ""
	}
	for i := uint32(0); i < body.ChildCount(); i++ {
		child := body.Child(i)
		if child.Type() == "string" || child.Type() == "concatenated_string" {
			return strings.Trim(ingestutil.NodeText(child, source), "\"'")
		}
		if child.Type() == "expression_statement" {
			if child.NamedChildCount() > 0 {
				expr := child.NamedChild(0)
				if expr.Type() == "string" || expr.Type() == "concatenated_string" {
					return strings.Trim(ingestutil.NodeText(expr, source), "\"'")
				}
			}
		}
		if child.IsNamed() {
			break
		}
	}
	return ""
}

// findDeclContaining returns the as-scope node whose name field starts at nameStart.
func findDeclContaining(policy PackQueries, lang string, root *sitter.Node, nameStart uint32) *sitter.Node {
	if root == nil || policy == nil || lang == "" {
		return nil
	}
	declTypes := map[string]bool{}
	for _, t := range policy.ScopeNodeTypes(lang) {
		declTypes[t] = true
	}
	if len(declTypes) == 0 {
		return nil
	}

	for i := uint32(0); i < root.ChildCount(); i++ {
		child := root.Child(i)
		if declTypes[child.Type()] {
			if n := ingestutil.ChildByField(child, "name"); n != nil && n.StartByte() == nameStart {
				return child
			}
		}
		if child.Type() == "export_statement" {
			for j := uint32(0); j < child.ChildCount(); j++ {
				inner := child.Child(j)
				if declTypes[inner.Type()] {
					if n := ingestutil.ChildByField(inner, "name"); n != nil && n.StartByte() == nameStart {
						return inner
					}
				}
			}
		}
		if found := findNestedMemberDecl(child, nameStart, declTypes); found != nil {
			return found
		}
	}
	return nil
}

func findNestedMemberDecl(n *sitter.Node, nameStart uint32, declTypes map[string]bool) *sitter.Node {
	if n == nil {
		return nil
	}
	if declTypes[n.Type()] {
		if name := ingestutil.ChildByField(n, "name"); name != nil && name.StartByte() == nameStart {
			return n
		}
	}
	body := ingestutil.ChildByField(n, "body")
	if body == nil {
		return nil
	}
	for i := uint32(0); i < body.ChildCount(); i++ {
		if found := findNestedMemberDecl(body.Child(i), nameStart, declTypes); found != nil {
			return found
		}
	}
	return nil
}

func extractSignature(funcNode *sitter.Node, source []byte) string {
	bodyNode := ingestutil.ChildByField(funcNode, "body")
	if bodyNode == nil {
		return ""
	}
	start := funcNode.StartByte()
	end := bodyNode.StartByte()
	if int(end) > len(source) {
		return ""
	}
	sig := strings.TrimSpace(string(source[start:end]))
	sig = strings.TrimSuffix(sig, ":")
	return strings.TrimSpace(sig)
}

// ExtractCommentBefore collects adjacent comment lines before a declaration.
// lineIsComment classifies a trimmed source line; languages supply style rules.
func ExtractCommentBefore(funcNode *sitter.Node, source []byte, lineIsComment func(string) bool) string {
	if funcNode == nil || lineIsComment == nil {
		return ""
	}
	funcStart := funcNode.StartByte()
	pos := int(funcStart) - 1
	for pos >= 0 && (source[pos] == ' ' || source[pos] == '\t' || source[pos] == '\n' || source[pos] == '\r') {
		pos--
	}
	if pos < 0 {
		return ""
	}

	lineEnd := pos + 1
	lineStart := pos
	for lineStart > 0 && source[lineStart-1] != '\n' {
		lineStart--
	}

	var lines []string
	for {
		line := strings.TrimSpace(string(source[lineStart:lineEnd]))
		if !lineIsComment(line) {
			break
		}
		cleaned := line
		cleaned = strings.TrimPrefix(cleaned, "//")
		cleaned = strings.TrimPrefix(cleaned, "/*")
		cleaned = strings.TrimSuffix(cleaned, "*/")
		cleaned = strings.TrimPrefix(cleaned, "*")
		cleaned = strings.TrimSpace(cleaned)
		lines = append([]string{cleaned}, lines...)

		lineEnd = lineStart
		if lineEnd <= 1 {
			break
		}
		lineEnd--
		lineStart = lineEnd
		for lineStart > 0 && source[lineStart-1] != '\n' {
			lineStart--
		}
	}
	return strings.Join(lines, "\n")
}
