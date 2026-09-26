package goref

import "errors"

// Tabled errors for ResolvePackageDir / go list.
var (
	ErrEmptyPackagePath = errors.New("go provider package path is empty")
	ErrPackageNotFound  = errors.New("go package not found")
	ErrCommandTimedOut  = errors.New("go command timed out")
	ErrEmptyListDir     = errors.New("go list returned empty directory")
	ErrInvalidListDir   = errors.New("go list directory is invalid")
)
