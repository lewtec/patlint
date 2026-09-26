package nixref

import "errors"

// Tabled errors for ResolveTarget / NIX_PATH lookup.
var (
	ErrEmptyPath    = errors.New("nix provider path is empty")
	ErrPathNotFound = errors.New("nix path not found")
)
