package project

// FamilyClaim is a pack as-family fact: host language joins this family.
type FamilyClaim struct {
	Lang   string
	Family string
	// Rules are as-family knobs on this claim. Family-wide resolve ORs
	// knobs from every claim with the same Family.
	Rules LanguageRules
}

// FamilyIDForLanguage is the as-family id for lang in claims, or "".
func FamilyIDForLanguage(claims []FamilyClaim, lang string) string {
	if lang == "" {
		return ""
	}
	for _, c := range claims {
		if c.Lang == lang {
			return c.Family
		}
	}
	return ""
}

// LanguagesInFamily returns language ids claimed for family (claim order).
func LanguagesInFamily(claims []FamilyClaim, family string) []string {
	if family == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, c := range claims {
		if c.Family != family || c.Lang == "" || seen[c.Lang] {
			continue
		}
		seen[c.Lang] = true
		out = append(out, c.Lang)
	}
	return out
}

// LanguageInFamily reports whether fileLanguage is a surface in projectFamily.
func LanguageInFamily(claims []FamilyClaim, fileLanguage, projectFamily string) bool {
	if fileLanguage == "" || projectFamily == "" {
		return false
	}
	if f := FamilyIDForLanguage(claims, fileLanguage); f != "" {
		return f == projectFamily
	}
	return fileLanguage == projectFamily
}

// SameFamily reports whether two language ids share a non-empty family.
func SameFamily(claims []FamilyClaim, a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	fa, fb := FamilyIDForLanguage(claims, a), FamilyIDForLanguage(claims, b)
	return fa != "" && fa == fb
}

// GrammarClaim is a pack as-grammar fact: host language parses with this grammar id.
type GrammarClaim struct {
	Lang    string
	Grammar string
}

// ImportLineClaim is a host (as-import (seq …)) fact.
type ImportLineClaim struct {
	Lang       string
	Tokens     []string
	HasLeaf    bool
	HasQual    bool
	QuotedPath bool
}

// PackageLineClaim is a host (as-package (seq …)) fact. pkg is the name field.
type PackageLineClaim struct {
	Lang   string
	Tokens []string
}

// AtomicSpanClaim is a host (as-atomic TEXT) fact: that source slice is one tape cell.
type AtomicSpanClaim struct {
	Lang string
	Text string
}

// DocstringClaim is a host (as-docstring KIND) fact.
type DocstringClaim struct {
	Lang string
	Kind string
}

// LayoutClaim is a host (as-layout NAME…) fact: file section order from the pack.
type LayoutClaim struct {
	Lang  string
	Names []string
}

// LanguageRules captures family resolve knobs. Product packs set them on
// (as-family ID KNOB…).
type LanguageRules struct {
	// DirectoryModule reports whether a directory can be treated as a symbol
	// container (for example path:./dir::Symbol).
	DirectoryModule bool
	// PackageScopedBareNames enables bare-name resolution across files that
	// share the same FileExtract.Package value (e.g. Go/Java packages).
	PackageScopedBareNames bool
	// EmptyPackageDirScoped, when PackageScopedBareNames is set and Package is
	// empty, limits peers to the same directory (Java default package).
	EmptyPackageDirScoped bool
	// NestedTypeMembers enables resolving bare leaves as Type.leaf using
	// enclosing type scope in the same file (enum constants / nested fields).
	NestedTypeMembers bool
	// IncludeFileExportsBare: a path-only import (no ::member) injects that
	// file's atoms into bare-name resolve (C/C++ #include).
	IncludeFileExportsBare bool
	// DirectoryManifest: a package.json (or other registered manifest) in the
	// directory names the backing file (Node main/exports).
	DirectoryManifest bool
	// ImportRefName: a rewrite ref's $path includes ::Name (Rust use paths).
	ImportRefName bool
	// DestExport: dest insert of a used atom gets an export prefix (ESM).
	DestExport bool
	// RejectDunderRename: reject rename of __name__ atoms.
	RejectDunderRename bool
}
