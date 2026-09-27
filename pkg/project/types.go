package project

// Result is the output of ingesting a directory of source files.
type Result struct {
	Files   []File  `json:"files"`
	Atoms   []Atom  `json:"atoms"`
	Aliases []Alias `json:"aliases,omitempty"`
	Uses    []Use   `json:"uses"`
	// Families are pack as-family claims from the VM that built this Result.
	Families []FamilyClaim `json:"-"`
}

// File records a source file and its language.
type File struct {
	Language string `json:"language"`
	Path     string `json:"path"`
	// Scopes are as-scope spans for this file (not in fixture JSON).
	Scopes []ScopeDef `json:"-"`
	// Flows are as-flow spans for this file (not in fixture JSON).
	Flows []FlowDef `json:"-"`
	// Package / PackageEnd are as-package (not in fixture JSON).
	Package    string `json:"-"`
	PackageEnd uint32 `json:"-"`
}

// Atom is a named symbol definition (function, class, type).
type Atom struct {
	Reference string `json:"reference"`
	StartByte uint32 `json:"start_byte"`
	EndByte   uint32 `json:"end_byte"`
	// Exported is omitted from fixture JSON. List hides false unless IncludeHidden.
	// Set at extract from as-atom public|private.
	Exported bool `json:"-"`
	// ScopeIdx is the innermost File.Scopes index; -1 is the file root.
	ScopeIdx int `json:"-"`
}

// Alias is an import binding that introduces a name in file scope.
type Alias struct {
	Reference string `json:"reference"`
	StartByte uint32 `json:"start_byte"`
	EndByte   uint32 `json:"end_byte"`
	Target    string `json:"target"`

	// as-import path token (not in fixture JSON). mv rewrites this span.
	ImportPath      string `json:"-"`
	ImportPathStart uint32 `json:"-"`
	ImportPathEnd   uint32 `json:"-"`
}

// Use is a usage of a symbol at a specific source location.
type Use struct {
	Reference      string `json:"reference"`
	StartByte      uint32 `json:"start_byte"`
	EndByte        uint32 `json:"end_byte"`
	Target         string `json:"target"`
	ViaImportAlias bool   `json:"via_import_alias,omitempty"`
}

// FileExtract holds raw facts from parsing a single source file.
type FileExtract struct {
	Language   string
	Path       string
	Package    string // Go package name; empty for Python/JS
	PackageEnd uint32 // end of as-package locus; 0 if unmarked

	Atoms     []AtomDef
	Imports   []ImportDef
	Usages    []UsageDef
	Reexports []ReexportDef
	// Scopes are as-scope spans. Nested by containment after extract.
	// Parent -1 is the implicit file root [0, len).
	Scopes []ScopeDef
	// Flows are as-flow spans (structural / hybrid). Not used by resolve.
	Flows []FlowDef
	// DefaultExport is the primary symbol this module exposes when imported as a
	// whole (no member). Empty if the driver did not identify a single primary export.
	DefaultExport string
}

// ReexportDef is a language-neutral "this module forwards X from Y" fact.
type ReexportDef struct {
	ExportName      string
	SourceName      string
	SourcePath      string
	Star            bool
	SourceStartByte uint32
	SourceEndByte   uint32
}

// FlowStructural / FlowHybrid are closed as-flow classes (SPEC Graph engine).
const (
	FlowStructural = "structural"
	FlowHybrid     = "hybrid"
)

// FlowDef is one control-flow increment from (as-flow CLASS MATCHER).
type FlowDef struct {
	StartByte uint32
	EndByte   uint32
	Class     string // FlowStructural | FlowHybrid
}

// ScopeDef is a span from (as-scope MATCHER) or (as-decl MATCHER).
// Parent is the nearest containing scope index, or -1 for the implicit file root.
// HoleOnly scopes are move holes (as-decl); RelationDeclares walks past them.
type ScopeDef struct {
	StartByte uint32
	EndByte   uint32
	Parent    int
	HoleOnly  bool
}

// AtomDef is a symbol definition found during extraction.
type AtomDef struct {
	Name      string
	StartByte uint32
	EndByte   uint32
	Exported  bool
	// ScopeIdx is the innermost covering Scopes index; -1 is the file root.
	ScopeIdx int
}

// ImportDef is an import declaration found during extraction.
type ImportDef struct {
	LocalName       string
	SourcePath      string
	MemberName      string
	StartByte       uint32
	EndByte         uint32
	TargetStartByte uint32
	TargetEndByte   uint32
	HasAliasBinding bool
	PathStartByte   uint32
	PathEndByte     uint32
}

// UsageName is one captured name span on a use path.
type UsageName struct {
	Name      string
	StartByte uint32
	EndByte   uint32
}

// UsageDef is a call-site identifier found during extraction.
type UsageDef struct {
	Scope     string
	Name      string
	StartByte uint32
	EndByte   uint32
	// Prefix is the name-path segments before the leaf (recv / pkg / ")").
	Prefix []UsageName
	// ScopeIdx is the innermost covering Scopes index; -1 is the file root.
	// Distinct from Scope, which is the as-use from-where name.
	ScopeIdx int
}
