package ingest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/datalog"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/reference"
	"github.com/lewtec/patlint/pkg/store"
)

// RenameFromResult adapts a Result snapshot into the store (LSP).
func RenameFromResult(ctx context.Context, sess *project.Session, result *project.Result, dir, sourceRef, destinationReference string, policy PackQueries) (project.Plan, error) {
	if result == nil {
		return project.Plan{}, fmt.Errorf("rename: nil Result")
	}
	st := store.New()
	store.LoadProjected(st, result)
	return RenameFromStore(ctx, sess, st, dir, sourceRef, destinationReference, policy)
}

// RenameFromStore plans identity rename / cross-file mv on a closed store.
func RenameFromStore(ctx context.Context, sess *project.Session, st *store.Store, dir, sourceRef, destinationReference string, policy PackQueries) (project.Plan, error) {
	if sess == nil {
		return project.Plan{}, ErrNilSession
	}
	if st == nil {
		return project.Plan{}, fmt.Errorf("rename: nil store")
	}
	view := store.Project(st, familiesOf(policy))
	src := ParseReference(sourceRef)
	dst := ParseReference(destinationReference)
	if (src.Name == "") != (dst.Name == "") {
		return project.Plan{}, ErrRenameSymbolMismatch
	}

	// CLI may pass absolute path: refs; store identity is ./rel under root.
	src = ProjectPathRef(dir, src)
	dst = ProjectPathRef(dir, dst)
	sourceRef = src.String()
	destinationReference = dst.String()

	slog.Debug("rename: store loaded",
		"files", len(view.Files),
		"atoms", len(view.Atoms),
		"uses", len(view.Uses),
		"aliases", len(view.Aliases),
	)

	src, err := CanonicalSourceReference(dir, view, src, policy)
	if err != nil {
		return project.Plan{}, err
	}
	dst, err = canonicalDestinationReference(dir, view, src, dst, policy)
	if err != nil {
		return project.Plan{}, err
	}
	src = ProjectPathRef(dir, src)
	dst = ProjectPathRef(dir, dst)
	slog.Debug("rename: canonical refs", "source", src.String(), "destination", dst.String())

	sourceRef = src.String()

	if src.Name == "" && dst.Name == "" {
		slog.Debug("rename: package move")
		return planPackageMove(ctx, dir, st, view, src, dst, policy)
	}

	if languageRules(policy, languageForRefPath(view, src.Path)).RejectDunderRename {
		oldLeaf := AtomName(src.Name)
		if len(oldLeaf) >= 5 && strings.HasPrefix(oldLeaf, "__") && strings.HasSuffix(oldLeaf, "__") {
			return project.Plan{}, fmt.Errorf("renaming Python special method %s is not supported", oldLeaf)
		}
	}

	sourceEntity, ok := findEntityByReference(view, sourceRef)
	if !ok {
		return project.Plan{}, fmt.Errorf("%w for reference %s", ErrEntityNotFound, sourceRef)
	}

	if src.Path != dst.Path {
		if src.Name != dst.Name {
			return project.Plan{}, ErrCrossFileRenameUnsupported
		}
		srcLang := languageForRefPath(view, src.Path)
		slog.Debug("rename: cross-file move", "lang", srcLang, "from", src.Path, "to", dst.Path)
		edits, err := planCrossFileMove(ctx, dir, st, view, src, dst, sourceEntity, policy)
		if err != nil {
			return project.Plan{}, err
		}
		return project.Plan{Edits: edits}, nil
	}

	sourceRefs := []string{sourceRef}
	slog.Debug("rename: symbol rename",
		"path", src.Path,
		"from", AtomName(src.Name),
		"to", AtomName(dst.Name),
		"source_refs", len(sourceRefs),
	)
	edits, files, err := planSymbolRename(ctx, policy, dir, st, view, sourceRefs, dst.Name)
	if err != nil {
		return project.Plan{}, err
	}
	return project.Plan{FileMoves: files, Edits: edits}, nil
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func findEntityByReference(result *project.Result, ref string) (project.Atom, bool) {
	for _, ent := range result.Atoms {
		if ent.Reference == ref {
			return ent, true
		}
	}
	return project.Atom{}, false
}

func planSymbolRename(ctx context.Context, policy PackQueries, dir string, st *store.Store, result *project.Result, sourceRefs []string, destinationSymbol string) ([]project.Edit, []project.FileMove, error) {
	if len(sourceRefs) == 0 {
		return nil, nil, ErrNoSourceRefsToRename
	}
	if st == nil {
		return nil, nil, fmt.Errorf("rename: nil store")
	}
	// Uses often target language providers (go:mod/pkg::Sym) while entities
	// are path:./pkg/file.go::Sym. Expand so cross-package qualified calls rename.
	sourceSet := expandRenameSourceSet(dir, result, sourceRefs)
	newText := AtomName(destinationSymbol)
	oldLeaf := AtomName(ParseReference(sourceRefs[0]).Name)
	slog.Debug("planSymbolRename", "source_set", len(sourceSet), "old_leaf", oldLeaf, "new_leaf", newText)

	want := map[string]string{}
	for ref := range sourceSet {
		if ref == "" {
			continue
		}
		want[ref] = newText
		want[canonicalRenameReference(ref)] = newText
	}
	st.Reset(store.RelationEdit)
	host := datalog.StoreHost{
		Store: st,
		RenameLeaf: func(ref string) (string, bool) {
			if n, ok := want[ref]; ok {
				return n, true
			}
			if n, ok := want[canonicalRenameReference(ref)]; ok {
				return n, true
			}
			return "", false
		},
	}
	if err := datalog.Eval(ctx, st, datalog.RenameProgram(), host); err != nil {
		return nil, nil, err
	}
	edits := editsFromStore(st)
	slog.Debug("planSymbolRename: spans", "total", len(edits))
	if len(edits) == 0 {
		return nil, nil, fmt.Errorf("%w for reference %s", ErrEntityNotFound, sourceRefs[0])
	}
	return deduplicateEdits(edits), nil, nil
}

func canonicalRenameReference(ref string) string {
	r := reference.NormalizePathReference(reference.Parse(ref))
	return r.String()
}

func editsFromStore(st *store.Store) []project.Edit {
	if st == nil {
		return nil
	}
	var out []project.Edit
	for _, t := range st.Rows(store.RelationEdit) {
		if len(t) < 4 {
			continue
		}
		sp := ingestutil.Span{StartByte: store.Atoi(t[1]), EndByte: store.Atoi(t[2])}
		if sp.StartByte > sp.EndByte {
			continue
		}
		if sp.Empty() && t[3] == "" {
			continue
		}
		out = append(out, project.Edit{
			File:    strings.TrimPrefix(t[0], "./"),
			Span:    sp,
			NewText: t[3],
		})
	}
	return out
}

func deduplicateEdits(edits []project.Edit) []project.Edit {
	type key struct {
		file       string
		start, end uint32
		text       string
	}
	seen := map[key]bool{}
	var out []project.Edit
	for _, e := range edits {
		k := key{e.File, e.StartByte, e.EndByte, e.NewText}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out
}

// expandRenameSourceSet adds language-provider targets (e.g. go:mod/pkg::Sym)
// that refer to the same package-scoped symbols as the path: entity refs.
// rootDir is the ingest root (passed to language PackageImportMatchers).
func expandRenameSourceSet(rootDir string, result *project.Result, sourceRefs []string) StringSet {
	sourceSet := NewStringSet()
	type want struct {
		packageDirectory string
		symbol           string
		fileStem         string // path basename without extension (setutils.py → setutils)
	}
	var wants []want
	seenWant := map[want]bool{}
	for _, s := range sourceRefs {
		if s == "" {
			continue
		}
		sourceSet.Add(s)
		ref := ParseReference(s)
		relPath := strings.TrimPrefix(ref.Path, "./")
		packageDirectory := path.Dir(relPath)
		if packageDirectory == "." {
			packageDirectory = ""
		}
		base := path.Base(relPath)
		stem := strings.TrimSuffix(base, path.Ext(base))
		if stem == "__init__" {
			// package dir: prefer parent segment as module leaf
			stem = path.Base(packageDirectory)
			if stem == "." || stem == "/" {
				stem = ""
			}
		}
		w := want{packageDirectory: packageDirectory, symbol: ref.Name, fileStem: stem}
		if ref.Name == "" || seenWant[w] {
			continue
		}
		seenWant[w] = true
		wants = append(wants, w)
	}
	if len(wants) == 0 || result == nil {
		return sourceSet
	}
	add := func(target string) {
		if target == "" || sourceSet.Has(target) {
			return
		}
		t := ParseReference(target)
		if t.Name == "" {
			return
		}
		// path: entities enter sourceSet only via sourceRefs.
		// Fan-out by bare leaf within the same directory rewrites unrelated
		// C++ members (DefaultOutputMgr.init → SearchState.init).
		if t.Provider == "path" || t.Provider == "" {
			return
		}
		for _, w := range wants {
			if t.Name != w.symbol {
				continue
			}
			if targetMatchesPackageSymbol(rootDir, t, w.packageDirectory) {
				sourceSet.Add(target)
				return
			}
			// Provider modules often use dotted import paths that PackageImportMatcher
			// cannot map for root-level files (packageDirectory ""). Match module leaf stem:
			// path:./setutils.py::f ↔ python:boltons.setutils::f
			if w.fileStem != "" {
				p := strings.Trim(t.Path, "/")
				if p == w.fileStem || strings.HasSuffix(p, "."+w.fileStem) {
					sourceSet.Add(target)
					return
				}
			}
		}
	}
	for _, rel := range result.Uses {
		add(rel.Target)
	}
	for _, a := range result.Aliases {
		add(a.Target)
	}
	expandTypeUseSameLeaf(result, sourceSet)
	return sourceSet
}

// expandTypeUseSameLeaf adds U.leaf when a use U → Type exists and Type.leaf
// is already in the set (implements / extends / new Type() { leaf }).
func expandTypeUseSameLeaf(result *project.Result, sourceSet StringSet) {
	if result == nil || sourceSet == nil {
		return
	}
	byName := map[string][]string{}
	for _, a := range result.Atoms {
		n := ParseReference(a.Reference).Name
		if n == "" {
			continue
		}
		byName[n] = append(byName[n], a.Reference)
	}
	for {
		n := sourceSet.Len()
		for src := range sourceSet {
			ref := ParseReference(src)
			typ, leaf := splitAtomTypeLeaf(ref.Name)
			if typ == "" || leaf == "" {
				continue
			}
			for _, u := range result.Uses {
				if ParseReference(u.Target).Name != typ {
					continue
				}
				from := ParseReference(u.Reference).Name
				if from == "" {
					continue
				}
				for _, r := range sameLeafOn(from, leaf, byName) {
					sourceSet.Add(r)
				}
			}
		}
		if sourceSet.Len() == n {
			return
		}
	}
}

// sameLeafOn is from.leaf, or Type.leaf when from is a nested name
// (use.Local → Box.Local.helper).
func sameLeafOn(from, leaf string, byName map[string][]string) []string {
	if rs := byName[from+"."+leaf]; len(rs) > 0 {
		return rs
	}
	fromLeaf := AtomName(from)
	if fromLeaf == "" || fromLeaf == from {
		return nil
	}
	exact := fromLeaf + "." + leaf
	suffix := "." + exact
	var out []string
	for name, rs := range byName {
		if name == exact || strings.HasSuffix(name, suffix) {
			out = append(out, rs...)
		}
	}
	return out
}

func splitAtomTypeLeaf(name string) (typ, leaf string) {
	leaf = AtomName(name)
	if leaf == "" || leaf == name {
		return "", leaf
	}
	return strings.TrimSuffix(name, "."+leaf), leaf
}

// targetMatchesPackageSymbol reports whether ref names a symbol in package packageDirectory
// (relative to the project root, e.g. "pkg/db").
// path: compares file directory. Other providers use PackageImportMatcher
// (language drivers; e.g. Go reads go.mod in ingest/go).
func targetMatchesPackageSymbol(rootDir string, ref Reference, packageDirectory string) bool {
	if ref.Provider == "path" || ref.Provider == "" {
		dir := path.Dir(strings.TrimPrefix(ref.Path, "./"))
		if dir == "." {
			dir = ""
		}
		return dir == packageDirectory
	}
	return packageImportIsPackage(rootDir, ref.Path, packageDirectory)
}

// AtomName returns the identifier text written at a definition/use span.
// Qualified symbols use "." separators; pointer receivers may prefix "*".
//
// String-keyed members keep their quotes in the symbol path (e.g. Type.'.md'
// for a TS property named '.md'). A naive last-dot split would peel inside the
// quotes ("md'"); treat a trailing single- or double-quoted segment as the leaf.
func AtomName(symbol string) string {
	leaf := symbol
	if q := quotedSymbolLeaf(leaf); q != "" {
		return q
	}
	if i := strings.LastIndex(leaf, "."); i >= 0 {
		leaf = leaf[i+1:]
	}
	return strings.TrimPrefix(leaf, "*")
}

// quotedSymbolLeaf returns a trailing '…' or "…" leaf when it is a full path
// segment (start of symbol or immediately after "."). Empty string means fall back.
func quotedSymbolLeaf(symbol string) string {
	if len(symbol) < 2 {
		return ""
	}
	q := symbol[len(symbol)-1]
	if q != '\'' && q != '"' {
		return ""
	}
	// Scan left for the matching opener; content may contain "." ('.md').
	for i := len(symbol) - 2; i >= 0; i-- {
		if symbol[i] != q {
			continue
		}
		if i == 0 || symbol[i-1] == '.' {
			return symbol[i:]
		}
		// Quote mid-segment (unlikely); keep scanning for an earlier opener.
	}
	return ""
}

// pathsArePairedSourceHeader reports whether a and b are the same basename stem
// with complementary C-family extensions (foo.h ↔ foo.cpp). Used to co-move
// free-function definitions with their header declarations.
func pathsArePairedSourceHeader(a, b string) bool {
	a = strings.TrimPrefix(a, "./")
	b = strings.TrimPrefix(b, "./")
	stem := func(p string) string {
		base := path.Base(p)
		return strings.TrimSuffix(base, path.Ext(base))
	}
	extOK := func(p string) bool {
		switch strings.ToLower(path.Ext(p)) {
		case ".c", ".h", ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx":
			return true
		default:
			return false
		}
	}
	return extOK(a) && extOK(b) && stem(a) == stem(b) && path.Ext(a) != path.Ext(b)
}

// mapPairedMovePath maps a paired unit path when its partner moves from sourcePath
// to destinationPath (Function.h → Sequence.h implies Function.cpp → Sequence.cpp).
func mapPairedMovePath(sourcePath, destinationPath, pairedPath string) string {
	sourcePath = strings.TrimPrefix(sourcePath, "./")
	destinationPath = strings.TrimPrefix(destinationPath, "./")
	pairedPath = strings.TrimPrefix(pairedPath, "./")
	srcStem := strings.TrimSuffix(path.Base(sourcePath), path.Ext(path.Base(sourcePath)))
	dstStem := strings.TrimSuffix(path.Base(destinationPath), path.Ext(path.Base(destinationPath)))
	pairedBase := path.Base(pairedPath)
	pairedExt := path.Ext(pairedBase)
	pairedStem := strings.TrimSuffix(pairedBase, pairedExt)
	if pairedStem != srcStem || srcStem == "" || dstStem == "" {
		return ""
	}
	newBase := dstStem + pairedExt
	dir := path.Dir(destinationPath)
	if dir == "." || dir == "" {
		return newBase
	}
	return path.Join(dir, newBase)
}

// rewriteQuotedIncludeInEdits rewrites #include "oldHeader" specs inside planned
// NewText bodies when a paired header co-moves (methods already inserted with
// the pre-move type header).
func rewriteQuotedIncludeInEdits(edits []project.Edit, oldHeader, newHeader string) []project.Edit {
	oldHeader = strings.TrimPrefix(oldHeader, "./")
	newHeader = strings.TrimPrefix(newHeader, "./")
	if oldHeader == "" || newHeader == "" || oldHeader == newHeader {
		return edits
	}
	oldBase := path.Base(oldHeader)
	newBase := path.Base(newHeader)
	// Prefer basename forms (same-dir includes are bare basenames).
	repls := []struct{ from, to string }{
		{`#include "` + oldHeader + `"`, `#include "` + newHeader + `"`},
		{`#include "` + oldBase + `"`, `#include "` + newBase + `"`},
	}
	if oldBase == newBase {
		return edits
	}
	out := make([]project.Edit, len(edits))
	copy(out, edits)
	for i := range out {
		if out[i].NewText == "" {
			continue
		}
		nt := out[i].NewText
		for _, r := range repls {
			nt = strings.ReplaceAll(nt, r.from, r.to)
		}
		// Co-move rewrites Type.h → Type_fuzz.h in InsertDecl imports while the
		// destination already #include "Type_fuzz.h", producing a duplicate line.
		out[i].NewText = deduplicateQuotedIncludeLines(nt)
	}
	return out
}

// deduplicateQuotedIncludeLines drops later exact-duplicate #include "…" lines
// (same trimmed text), keeping the first. System includes are left alone.
func deduplicateQuotedIncludeLines(content string) string {
	if content == "" || !strings.Contains(content, "#include") {
		return content
	}
	lines := strings.Split(content, "\n")
	seen := map[string]bool{}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#include") && strings.Contains(trimmed, `"`) {
			if seen[trimmed] {
				continue
			}
			seen[trimmed] = true
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// planCrossFileMove extracts the covering scope and inserts it at dest.
// It extracts the declaration, inserts it at the destination, and if the source
// and destination are in different directories, rewrites imports in consumer files.
func planCrossFileMove(ctx context.Context, dir string, st *store.Store, result *project.Result, src, destination Reference, sourceEntity project.Atom, policy PackQueries) ([]project.Edit, error) {
	if st == nil {
		return nil, fmt.Errorf("rename: nil store")
	}
	sourceRelative := strings.TrimPrefix(src.Path, "./")
	destinationRelative := strings.TrimPrefix(destination.Path, "./")

	decl, err := extractDeclarationFromResult(dir, result, sourceEntity)
	if err != nil {
		return nil, err
	}
	if err := rejectMovedAtom(dir, result, src, destination, sourceEntity, decl, policy); err != nil {
		return nil, err
	}
	if err := rejectCircularResidualMove(dir, result, src, destination, sourceEntity, decl); err != nil {
		return nil, err
	}
	destImport := newMoveQualify(dir, result, st, src, destination, sourceEntity, &decl, policy)
	lang := destImport.language
	importLine := destImport.pack.importLine
	if languageRules(policy, lang).DestExport && destinationNeedsExport(result, src, destinationRelative, sourceEntity, decl) {
		decl.DeclText = ensureExportPrefix(decl.DeclText)
	}

	destinationPath := lewpath.New(dir, destinationRelative).String()
	destinationContent, err := os.ReadFile(destinationPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			destinationContent = nil
		} else {
			return nil, fmt.Errorf("reading %s: %w", destinationRelative, err)
		}
	}
	if importLine.HasQual && len(destinationContent) == 0 {
		if left := leftoverSourceNames(result, sourceRelative, sourceEntity, decl); len(left) > 0 {
			decl.DeclText = qualifyIdentifiersInText(decl.DeclText, left, fileStem(sourceRelative))
		}
	}
	prepended := false
	if importLine.HasQual && len(destinationContent) > 0 {
		if block := destImport.holeDependencyBlock(); block != "" {
			decl.DeclText = strings.TrimRight(block, "\n") + "\n\n" + decl.DeclText
			prepended = true
		}
	}

	insertEdit := insertDeclarationGeneric(destinationRelative, destinationContent, decl, policy, result)
	if prepended && strings.HasPrefix(insertEdit.NewText, "\n") {
		insertEdit.NewText = insertEdit.NewText[1:]
	}
	oldKey, newKey := importRewriteKeys(src, destination)
	st.Reset(store.RelationEdit)
	host := datalog.StoreHost{
		Store: st,
		MoveHole: [][]string{{
			sourceRelative, store.Itoa(decl.RemoveStart), store.Itoa(decl.RemoveEnd),
		}},
		MovePlace: [][]string{{
			destinationRelative, store.Itoa(insertEdit.StartByte), store.Itoa(insertEdit.EndByte), insertEdit.NewText,
		}},
		RewriteSpec: func(file, specifier string) (string, bool) {
			if languageRules(policy, languageForRefPath(result, sourceRelative)).PackageScopedBareNames {
				if fileKeepsOldPackage(result, file, sourceRelative, sourceEntity) {
					return "", false
				}
			} else if importKeepsOtherMembers(dir, result, file, specifier, sourceRelative, AtomName(src.Name)) ||
				fileKeepsOldPackage(result, file, sourceRelative, sourceEntity) {
				return "", false
			}
			if np := RewriteImportPathFile(file, specifier, sourceRelative, destinationRelative, policy); np != "" && np != specifier {
				return np, true
			}
			np := RewriteImportPathDir(specifier, oldKey, newKey)
			if np == "" || np == specifier {
				return "", false
			}
			return np, true
		},
		NotMoveFile: func(file string) bool {
			f := strings.TrimPrefix(file, "./")
			return f != sourceRelative && f != destinationRelative
		},
	}
	if err := datalog.Eval(ctx, st, datalog.MoveProgram(), host); err != nil {
		return nil, err
	}
	qualifyEdits, err := destImport.edits(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range qualifyEdits {
		st.Insert(store.RelationEdit, store.Tuple{e.File, store.Itoa(e.StartByte), store.Itoa(e.EndByte), e.NewText})
	}
	if srcBytes, err := os.ReadFile(lewpath.New(dir, sourceRelative).String()); err == nil {
		fe := fileExtractForImports(result, sourceRelative, st)
		decl.Imports = holeImportCandidates(fe, decl, srcBytes)
		for _, e := range PruneNamedUnusedForDecl(sourceRelative, srcBytes, fe, decl) {
			st.Insert(store.RelationEdit, store.Tuple{e.File, store.Itoa(e.StartByte), store.Itoa(e.EndByte), e.NewText})
		}
	}
	return deduplicateEdits(mergeDestinationImportHeader(editsFromStore(st), destinationRelative)), nil
}

func mergeDestinationImportHeader(edits []project.Edit, destinationRelative string) []project.Edit {
	destinationRelative = strings.TrimPrefix(destinationRelative, "./")
	var header string
	var hole []project.Edit
	var rest []project.Edit
	for _, e := range edits {
		if strings.TrimPrefix(e.File, "./") != destinationRelative {
			rest = append(rest, e)
			continue
		}
		t := strings.TrimLeft(e.NewText, "\n")
		if (strings.HasPrefix(t, "import \"") || strings.HasPrefix(t, "import (")) && !strings.Contains(t, "func ") && !strings.Contains(t, "type ") {
			header += t
			continue
		}
		hole = append(hole, e)
	}
	if header == "" || len(hole) == 0 {
		return edits
	}
	for i := range hole {
		hole[i].NewText = spliceAfterPackage(hole[i].NewText, header)
	}
	return append(rest, hole...)
}

func spliceAfterPackage(body, header string) string {
	header = strings.TrimSpace(header) + "\n"
	if strings.HasPrefix(body, "package ") {
		i := strings.IndexByte(body, '\n')
		if i < 0 {
			return body + "\n\n" + header
		}
		return body[:i+1] + "\n\n" + header + "\n" + strings.TrimLeft(body[i+1:], "\n")
	}
	return "\n\n" + header + body
}

func destinationNeedsExport(result *project.Result, src Reference, destinationRelative string, entity project.Atom, hole DeclExtract) bool {
	if result == nil {
		return false
	}
	destinationRelative = strings.TrimPrefix(destinationRelative, "./")
	for _, u := range result.Uses {
		if !useTargetsMovedAtom(u, src, entity) {
			continue
		}
		fileRel := strings.TrimPrefix(ParseReference(u.Reference).Path, "./")
		if fileRel == destinationRelative {
			continue
		}
		if fileRel == strings.TrimPrefix(src.Path, "./") && u.StartByte >= hole.RemoveStart && u.EndByte <= hole.RemoveEnd {
			continue
		}
		return true
	}
	return false
}

func ensureExportPrefix(text string) string {
	i := 0
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	if strings.HasPrefix(text[i:], "export ") {
		return text
	}
	return text[:i] + "export " + text[i:]
}

func holeImportCandidates(fe *project.FileExtract, hole DeclExtract, src []byte) []string {
	if fe == nil {
		return nil
	}
	var out []string
	for _, im := range fe.Imports {
		local := im.LocalName
		if local == "" {
			local = im.MemberName
		}
		locals := namedImportLocals(im, src)
		used := false
		for _, n := range locals {
			if containsIdentifier([]byte(hole.DeclText), n) {
				used = true
				break
			}
		}
		if !used && im.SourcePath != "" && containsIdentifier([]byte(hole.DeclText), LastPathComponent(im.SourcePath)) {
			used = true
		}
		if !used && (local == "" || !containsIdentifier([]byte(hole.DeclText), local)) {
			continue
		}
		if im.EndByte > im.StartByte && int(im.EndByte) <= len(src) {
			out = append(out, string(src[im.StartByte:im.EndByte]))
		}
		if im.SourcePath != "" {
			out = append(out, im.SourcePath)
		}
	}
	return out
}

// planCrossFileMoveImportRewrites finds all consumer files that import the
// source module/package and rewrites them to point to the destination.
func planCrossFileMoveImportRewrites(dir string, result *project.Result, src, destination Reference, policy PackQueries) ([]project.Edit, error) {
	// Build the set of reference strings that all point at the source entity/file.
	// Different languages resolve imports to different provider namespaces
	// (e.g. path:./utils.py vs python:fastapi.utils), so we need to match
	// against every form the source might appear as in alias/relation targets.
	srcTargets := buildSourceTargetSet(result, src)

	// Find files that have aliases targeting the source symbol or source module.
	consumerFiles := map[string]bool{}
	for _, alias := range result.Aliases {
		if srcTargets[alias.Target] {
			ref := ParseReference(alias.Reference)
			consumerFile := strings.TrimPrefix(ref.Path, "./")
			consumerFiles[consumerFile] = true
		}
	}
	// Also check relations targeting the source.
	for _, rel := range result.Uses {
		if srcTargets[rel.Target] {
			ref := ParseReference(rel.Reference)
			consumerFile := strings.TrimPrefix(ref.Path, "./")
			consumerFiles[consumerFile] = true
		}
	}

	sourceRelative := strings.TrimPrefix(src.Path, "./")
	destinationRelative := strings.TrimPrefix(destination.Path, "./")

	var edits []project.Edit
	for consumerFile := range consumerFiles {
		// Don't rewrite the source or destination files themselves.
		if consumerFile == sourceRelative || consumerFile == destinationRelative {
			continue
		}

		content, err := os.ReadFile(lewpath.New(dir, consumerFile).String())
		if err != nil {
			continue
		}

		occs := RewriteImportsInFile(policy, consumerFile, content, result, src.String(), destination.String())
		edits = append(edits, occs...)
	}

	for _, f := range result.Files {
		rel := strings.TrimPrefix(f.Path, "./")
		if rel == sourceRelative || rel == destinationRelative || consumerFiles[rel] {
			continue
		}
		content, err := os.ReadFile(lewpath.New(dir, rel).String())
		if err != nil {
			continue
		}
		occs := RewriteImportsInFile(policy, rel, content, result, src.String(), destination.String())
		edits = append(edits, occs...)
	}

	return edits, nil
}

// importRewriteLanguageMatch reports whether a consumer file language should be
// scanned for import rewrites driven by a move in driverLanguage.
// Same-family surfaces share import syntax (C/C++ #include, ECMA specifiers).
func importRewriteLanguageMatch(claims []project.FamilyClaim, fileLanguage, driverLanguage string) bool {
	return project.SameFamily(claims, fileLanguage, driverLanguage)
}

// buildSourceTargetSet builds a set of reference strings that might appear as
// alias or relation targets for the given source reference. Because different
// languages resolve imports into different provider namespaces (e.g.
// "path:./utils.py::X" vs "python:fastapi.utils::X"), we scan the existing
// result for any target that points at the same file, with or without the
// symbol qualifier.
func buildSourceTargetSet(result *project.Result, src Reference) map[string]bool {
	srcRef := src.String()
	srcFileRef := FileRef(src.Path)
	srcDirRef := FileRef("./" + path.Dir(strings.TrimPrefix(src.Path, "./")))
	sourceRelative := strings.TrimPrefix(src.Path, "./")

	targets := map[string]bool{
		srcRef:     true,
		srcFileRef: true,
		srcDirRef:  true,
	}

	// Scan all aliases and relations to find targets that reference the same
	// file (by any provider namespace). We match on file path suffix to
	// catch e.g. "python:fastapi.utils" mapping to file "utils.py".
	for _, alias := range result.Aliases {
		ref := ParseReference(alias.Target)
		if ref.Provider == "path" {
			continue // already covered by the direct matches above
		}
		if aliasTargetMatchesFile(result, alias.Target, ref, sourceRelative, src.Name) {
			targets[alias.Target] = true
		}
	}
	for _, rel := range result.Uses {
		ref := ParseReference(rel.Target)
		if ref.Provider == "path" {
			continue
		}
		if aliasTargetMatchesFile(result, rel.Target, ref, sourceRelative, src.Name) {
			targets[rel.Target] = true
		}
	}

	return targets
}

// aliasTargetMatchesFile checks if a non-path target reference actually points
// at the given source file. It does this by checking if the target entity
// exists among the entities declared in the source file.
func aliasTargetMatchesFile(result *project.Result, targetStr string, targetRef Reference, sourceFileRelative, sourceSymbol string) bool {
	if sourceSymbol != "" {
		// For symbol-level matches, check if there's an entity in the source
		// file with matching symbol name.
		wantEntRef := AtomRef("./"+sourceFileRelative, sourceSymbol)
		for _, ent := range result.Atoms {
			if ent.Reference == wantEntRef {
				// Now check: does targetRef reference the same symbol?
				if targetRef.Name == sourceSymbol {
					return true
				}
			}
		}
		return false
	}

	// For file/dir level matches, check if any alias or entity from the
	// source file has a target that resolves to this reference.
	srcFilePath := "./" + sourceFileRelative
	for _, alias := range result.Aliases {
		ref := ParseReference(alias.Reference)
		if ref.Path == srcFilePath && alias.Target == targetStr {
			return true
		}
	}
	return false
}

// FindAllWholeWordOccurrences finds all whole-word occurrences of oldBase in
// content and returns edits to replace them with newBase. A "whole word" match
// means the match is not preceded or followed by a letter, digit or underscore.
func FindAllWholeWordOccurrences(file string, content []byte, oldBase, newBase string) []project.Edit {
	if oldBase == "" || oldBase == newBase {
		return nil
	}
	text := string(content)
	var edits []project.Edit
	off := 0
	for {
		idx := strings.Index(text[off:], oldBase)
		if idx < 0 {
			break
		}
		pos := off + idx
		endPos := pos + len(oldBase)

		// Check word boundaries.
		if pos > 0 && isWordCharacter(text[pos-1]) {
			off = endPos
			continue
		}
		if endPos < len(text) && isWordCharacter(text[endPos]) {
			off = endPos
			continue
		}

		edits = append(edits, project.Edit{
			File:    file,
			Span:    ingestutil.Span{StartByte: uint32(pos), EndByte: uint32(endPos)},
			NewText: newBase,
		})
		off = endPos
	}
	return edits
}

func isWordCharacter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// FindAllOccurrences finds all occurrences of oldBase in content and returns
// edits to replace them with newBase.
func FindAllOccurrences(file string, content []byte, oldBase, newBase string) []project.Edit {
	if oldBase == "" || oldBase == newBase {
		return nil
	}
	text := string(content)
	var edits []project.Edit
	off := 0
	for {
		idx := strings.Index(text[off:], oldBase)
		if idx < 0 {
			break
		}
		pos := off + idx
		edits = append(edits, project.Edit{
			File:    file,
			Span:    ingestutil.Span{StartByte: uint32(pos), EndByte: uint32(pos + len(oldBase))},
			NewText: newBase,
		})
		off = pos + len(oldBase)
	}
	return edits
}

// FindAllOccurrencesInStrings is like FindAllOccurrences but only produces
// edits for matches that occur inside double-quoted or raw string literals.
// Language packages use this for import-path text; language-specific boundary
// rules belong in those packages.
func FindAllOccurrencesInStrings(file string, content []byte, oldBase, newBase string) []project.Edit {
	if oldBase == "" || oldBase == newBase {
		return nil
	}
	var edits []project.Edit
	ingestutil.ForEachStringLiteral(content, func(seg string, start int) bool {
		sOff := 0
		for {
			idx := strings.Index(seg[sOff:], oldBase)
			if idx < 0 {
				break
			}
			posInSeg := sOff + idx
			pos := start + posInSeg
			edits = append(edits, project.Edit{
				File:    file,
				Span:    ingestutil.Span{StartByte: uint32(pos), EndByte: uint32(pos + len(oldBase))},
				NewText: newBase,
			})
			sOff = posInSeg + len(oldBase)
		}
		return true
	})
	return edits
}

// CommonPathPrefix returns the common directory prefix of two paths.
func CommonPathPrefix(a, b string) string {
	aa := strings.Split(strings.Trim(a, "/"), "/")
	bb := strings.Split(strings.Trim(b, "/"), "/")
	n := len(aa)
	if len(bb) < n {
		n = len(bb)
	}
	var p []string
	for i := 0; i < n; i++ {
		if aa[i] != bb[i] {
			break
		}
		p = append(p, aa[i])
	}
	if len(p) == 0 {
		return ""
	}
	return strings.Join(p, "/") + "/"
}
