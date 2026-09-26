package ingest

import (
	"errors"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"
)

// Tabled package errors for %w wrapping and errors.Is checks.
var (
	ErrEntityNotFound             = errors.New("entity not found")
	ErrProviderDocNeedsSymbol     = errors.New("provider doc reference requires symbol (::name)")
	ErrUnsupportedLanguage        = ingestutil.ErrUnsupportedLanguage
	ErrAmbiguousSymbol            = errors.New("ambiguous symbol")
	ErrAmbiguousDirectoryRef      = errors.New("ambiguous directory reference")
	ErrDirectorySymbolUnsupported = errors.New("directory symbol reference is not supported for this language; use a file reference")
	ErrDirectoryDestUnsupported   = errors.New("directory destination is not supported for this language; use a file path")
	ErrDeclarationNotFound        = errors.New("declaration not found")
	ErrSourceLanguageUnknown      = errors.New("could not determine language for source")
	ErrNoDirectoryDestMapping     = errors.New("driver does not provide directory destination mapping")
	ErrListingNotSupported        = errors.New("listing not supported for provider")
	ErrEditOutOfBounds            = project.ErrEditOutOfBounds
	ErrWalkNilYield               = errors.New("WalkExtracts: nil yield")
	ErrWalkUnknownKind            = errors.New("WalkExtracts: unknown kind")
	ErrNilSession                 = errors.New("ingest: nil Session (caller must pass *Session)")
	ErrNilPolicy                  = errors.New("ingest: nil PackQueries (caller must pass the VM)")
	ErrProviderScopeEmptyDir      = errors.New("provider scope: empty dir")
	ErrRenameSymbolMismatch       = errors.New("source and destination references must both include symbols or both omit them (for package moves)")
	ErrCrossFileRenameUnsupported = errors.New("cross-file move with symbol rename is not supported yet")
	ErrCrossFileMoveUnsupported   = errors.New("cross-file move is not supported for language")
	ErrNoSourceRefsToRename       = errors.New("no source references to rename")
	ErrDirMoveEmptyPaths          = errors.New("dir move requires non-empty from/to")
	ErrPackageMoveEmptyPaths      = errors.New("package move requires non-empty directory paths")
	ErrSetLanguage                = ingestutil.ErrSetLanguage
	ErrTreeSitterFault            = errors.New("tree-sitter fault")
	ErrStagedBadSpan              = errors.New("staged bad span")
	ErrStagedEntityMismatch       = errors.New("staged entity text mismatch")
	ErrJoinFamily                 = errors.New("family join")
	ErrUnknownFamily              = errors.New("unknown family")
	ErrLanguageAlreadyInFamily    = errors.New("language already in family")
)
