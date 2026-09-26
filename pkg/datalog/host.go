package datalog

import (
	"strconv"
	"strings"

	"github.com/lewtec/patlint/pkg/reference"
	"github.com/lewtec/patlint/pkg/store"
)

// StoreHost is builtins that only need the store + import resolve.
type StoreHost struct {
	Store         *store.Store
	Root          string
	KnownFiles    map[string]bool
	KnownDirs     map[string]bool
	ResolveImport func(spec, importer, root string, files, dirs map[string]bool) string
	// RenameLeaf is the identity-rename goal: ref → new identifier text.
	RenameLeaf func(ref string) (string, bool)
	// MoveHole / MovePlace are cross-file mv tuples (file, start, end[, text]).
	MoveHole  [][]string
	MovePlace [][]string
	// RewriteSpec maps a consumer file + marked import specifier to its replacement.
	RewriteSpec func(file, spec string) (string, bool)
	// NotMoveFile is true when file may receive import-path rewrites.
	NotMoveFile func(file string) bool
}

// Builtin implements Host.
func (h StoreHost) Builtin(name string, args []string) [][]string {
	switch name {
	case "mod_resolve":
		if len(args) < 2 || h.ResolveImport == nil {
			return nil
		}
		t := h.ResolveImport(args[0], args[1], h.Root, h.KnownFiles, h.KnownDirs)
		if t == "" {
			return nil
		}
		return [][]string{{t}}
	case "file_ref":
		if len(args) < 1 {
			return nil
		}
		return [][]string{{reference.FileRef(pathDot(args[0]))}}
	case "file_ref_of":
		if len(args) < 1 {
			return nil
		}
		r := reference.Parse(args[0])
		if r.Provider == "" {
			r.Provider = "path"
		}
		r.Name = ""
		if r.Provider == "path" {
			r = reference.NormalizePathReference(r)
		}
		return [][]string{{r.String()}}
	case "atom_ref":
		if len(args) < 2 {
			return nil
		}
		return [][]string{{reference.AtomRef(pathDot(args[0]), args[1])}}
	case "import_member":
		if len(args) < 2 {
			return nil
		}
		return [][]string{{importMember(h.Store, args[0], args[1])}}
	case "pick_span":
		if len(args) < 4 {
			return nil
		}
		s, e, ts, te := args[0], args[1], args[2], args[3]
		if ts != "0" || te != "0" {
			return [][]string{{ts, te}}
		}
		return [][]string{{s, e}}
	case "reexport_names":
		if len(args) < 2 {
			return nil
		}
		ex, sn := args[0], args[1]
		if ex == "" {
			ex = sn
		}
		if sn == "" {
			sn = ex
		}
		if ex == "" {
			return nil
		}
		return [][]string{{ex, sn}}
	case "reexport_target":
		if len(args) < 2 {
			return nil
		}
		base, srcn := args[0], args[1]
		r := reference.Parse(base)
		if r.Provider == "" {
			r.Provider = "path"
		}
		if srcn == "" {
			r.Name = ""
			return [][]string{{r.String()}}
		}
		return [][]string{{reference.AtomRef(pathDot(r.Path), srcn)}}
	case "use_from":
		if len(args) < 2 {
			return nil
		}
		return [][]string{{useFrom(h.Store, args[0], args[1])}}
	case "visible_at":
		if len(args) < 5 {
			return nil
		}
		t := visibleAt(h.Store, args[0], store.AtoiInt(args[1]), args[4])
		if t == "" {
			return nil
		}
		return [][]string{{t}}
	case "concat":
		if len(args) < 3 {
			return nil
		}
		return [][]string{{args[0] + args[1] + args[2]}}
	case "leaf_eq":
		if len(args) < 2 {
			return nil
		}
		if atomLeaf(args[0]) != args[1] && args[0] != args[1] {
			return nil
		}
		return [][]string{{}}
	case "neq":
		if len(args) < 2 || args[0] == args[1] {
			return nil
		}
		return [][]string{{}}
	case "ge0":
		if len(args) < 1 {
			return nil
		}
		n, _ := strconv.Atoi(args[0])
		if n < 0 {
			return nil
		}
		return [][]string{{}}
	case "gt0":
		if len(args) < 1 {
			return nil
		}
		n, _ := strconv.Atoi(args[0])
		if n <= 0 {
			return nil
		}
		return [][]string{{}}
	case "pred":
		if len(args) < 1 {
			return nil
		}
		n, _ := strconv.Atoi(args[0])
		if n <= 0 {
			return nil
		}
		return [][]string{{strconv.Itoa(n - 1)}}
	case "last":
		if len(args) < 1 {
			return nil
		}
		n, _ := strconv.Atoi(args[0])
		if n <= 0 {
			return nil
		}
		return [][]string{{strconv.Itoa(n - 1)}}
	case "path_file":
		if len(args) < 1 {
			return nil
		}
		r := reference.Parse(args[0])
		if r.Provider != "path" && r.Provider != "" {
			return nil
		}
		if r.Name != "" {
			return nil
		}
		p := r.Path
		if !strings.HasPrefix(p, "./") && !strings.HasPrefix(p, "../") && !strings.HasPrefix(p, "/") {
			p = "./" + p
		}
		return [][]string{{p}}
	case "pkg_peer":
		if len(args) < 4 {
			return nil
		}
		f, g, pkg, lang := args[0], args[1], args[2], args[3]
		if !h.Store.Contains(store.RelationLanguage, store.Tuple{lang, store.FlagEmptyDir}) {
			return [][]string{{}}
		}
		if pkg != "" {
			return [][]string{{}}
		}
		if dirOf(f) == dirOf(g) {
			return [][]string{{}}
		}
		return nil
	case "not_private":
		if len(args) < 1 || strings.HasPrefix(args[0], "_") {
			return nil
		}
		return [][]string{{}}
	case "hop":
		if len(args) < 2 {
			return nil
		}
		t := hop(h.Store, args[0], args[1])
		if t == "" {
			return nil
		}
		return [][]string{{t}}
	case "is_named":
		if len(args) < 1 || !isNamedLocal(args[0]) {
			return nil
		}
		return [][]string{{}}
	case "import_name":
		if len(args) < 2 {
			return nil
		}
		return [][]string{{importName(args[0], args[1])}}
	case "import_span":
		if len(args) < 4 {
			return nil
		}
		as, ae, ok := importSpan(args[0], args[1], args[2], args[3])
		if !ok {
			return nil
		}
		return [][]string{{as, ae}}
	case "outside_import":
		if len(args) < 3 || !outsideImport(h.Store, args[0], args[1], args[2]) {
			return nil
		}
		return [][]string{{}}
	case "rename_leaf":
		if len(args) < 1 || h.RenameLeaf == nil {
			return nil
		}
		n, ok := h.RenameLeaf(args[0])
		if !ok || n == "" {
			return nil
		}
		return [][]string{{n}}
	case "ref_file":
		if len(args) < 1 {
			return nil
		}
		p := refFile(args[0])
		if p == "" {
			return nil
		}
		return [][]string{{p}}
	case "span_ok":
		if len(args) < 2 || store.Atoi(args[1]) <= store.Atoi(args[0]) {
			return nil
		}
		return [][]string{{}}
	case "move_hole":
		return h.MoveHole
	case "move_place":
		return h.MovePlace
	case "rewrite_spec":
		if h.RewriteSpec == nil {
			return nil
		}
		file, spec := "", ""
		if len(args) >= 2 {
			file, spec = args[0], args[1]
		} else if len(args) == 1 {
			spec = args[0]
		} else {
			return nil
		}
		n, ok := h.RewriteSpec(file, spec)
		if !ok || n == "" || n == spec {
			return nil
		}
		return [][]string{{n}}
	case "not_move_file":
		if len(args) < 1 {
			return nil
		}
		if h.NotMoveFile != nil && !h.NotMoveFile(args[0]) {
			return nil
		}
		return [][]string{{}}
	default:
		return nil
	}
}

func importMember(st *store.Store, base, mem string) string {
	if mem == "" {
		return base
	}
	r := reference.Parse(base)
	file := strings.TrimPrefix(r.Path, "./")
	if r.Provider == "path" || r.Provider == "" {
		if t, ok := atomNamed(st, file, mem); ok {
			return t
		}
		if t, ok := atomLeafIn(st, file, mem); ok {
			return t
		}
	}
	return base + "::" + mem
}

func atomNamed(st *store.Store, file, name string) (string, bool) {
	if st == nil {
		return "", false
	}
	want := pathDot(file)
	for _, t := range st.Rows(store.RelationAtomInFile) {
		if len(t) >= 3 && (t[0] == want || t[0] == file) && t[1] == name {
			return t[2], true
		}
	}
	return "", false
}

func atomLeafIn(st *store.Store, file, leaf string) (string, bool) {
	if st == nil || leaf == "" {
		return "", false
	}
	want := pathDot(file)
	suf := "." + leaf
	for _, t := range st.Rows(store.RelationAtom) {
		if len(t) < 2 {
			continue
		}
		if t[0] != want && t[0] != file {
			continue
		}
		if t[1] == leaf || strings.HasSuffix(t[1], suf) {
			return reference.AtomRef(pathDot(t[0]), t[1]), true
		}
	}
	return "", false
}

func useFrom(st *store.Store, file, scope string) string {
	if scope == "" {
		return reference.FileRef(pathDot(file))
	}
	if t, ok := atomNamed(st, file, scope); ok {
		return t
	}
	var found string
	n := 0
	leaf := scope
	for _, t := range st.Rows(store.RelationAtom) {
		if len(t) < 2 || (t[0] != file && t[0] != pathDot(file)) {
			continue
		}
		if atomLeaf(t[1]) != leaf {
			continue
		}
		n++
		found = t[1]
	}
	if n == 1 {
		return reference.AtomRef(pathDot(file), found)
	}
	return reference.AtomRef(pathDot(file), scope)
}

func visibleAt(st *store.Store, file string, scopeIdx int, name string) string {
	if st == nil || name == "" {
		return ""
	}
	// exact walk: this scope, then parents via RelationVisible already closed
	idx := strconv.Itoa(scopeIdx)
	for _, t := range st.Rows(store.RelationVisible) {
		if len(t) >= 4 && (t[0] == file || t[0] == pathDot(file)) && t[1] == idx && t[2] == name {
			return t[3]
		}
	}
	if scopeIdx >= 0 {
		for _, t := range st.Rows(store.RelationVisible) {
			if len(t) >= 4 && (t[0] == file || t[0] == pathDot(file)) && t[1] == "-1" && t[2] == name {
				return t[3]
			}
		}
	}
	return ""
}

func hop(st *store.Store, base, member string) string {
	if member == "" {
		return ""
	}
	r := reference.Parse(base)
	file := strings.TrimPrefix(r.Path, "./")
	if r.Name != "" && (r.Provider == "path" || r.Provider == "") {
		qual := r.Name + "." + member
		if t, ok := atomNamed(st, file, qual); ok {
			return t
		}
		return base + "::" + member
	}
	if r.Provider == "path" || r.Provider == "" {
		if t, ok := atomNamed(st, file, member); ok {
			return t
		}
	}
	return base + "::" + member
}

func atomLeaf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

func pathDot(rel string) string {
	if strings.HasPrefix(rel, "./") || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
		return rel
	}
	return "./" + rel
}

func dirOf(p string) string {
	p = strings.TrimPrefix(p, "./")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

func isNamedLocal(name string) bool {
	return name != "" && name != "*" && name != "." && name != "_"
}

func importName(local, spec string) string {
	if isNamedLocal(local) {
		return local
	}
	spec = strings.Trim(spec, `"'`+"`")
	if i := strings.LastIndex(spec, "/"); i >= 0 {
		return spec[i+1:]
	}
	return spec
}

func importSpan(s, e, ps, pe string) (string, string, bool) {
	if store.Atoi(e) > store.Atoi(s) {
		return s, e, true
	}
	if store.Atoi(pe) > store.Atoi(ps) {
		return ps, pe, true
	}
	return "", "", false
}

func refFile(ref string) string {
	r := reference.Parse(ref)
	if r.Provider != "path" && r.Provider != "" {
		return ""
	}
	if r.Path == "" {
		return ""
	}
	return pathDot(r.Path)
}

func outsideImport(st *store.Store, file, us, ue string) bool {
	if st == nil {
		return true
	}
	u0, u1 := store.Atoi(us), store.Atoi(ue)
	for _, t := range st.Rows(store.RelationImport) {
		if len(t) < 12 || (t[0] != file && t[0] != pathDot(file) && pathDot(t[0]) != file) {
			continue
		}
		s, e := store.Atoi(t[4]), store.Atoi(t[5])
		if e <= s {
			s, e = store.Atoi(t[9]), store.Atoi(t[10])
		}
		if e <= s {
			continue
		}
		if u0 >= s && u1 <= e {
			return false
		}
	}
	return true
}
