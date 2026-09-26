package javaref

import "errors"

// Tabled errors for ResolvePackageDir / ResolveModuleTarget.
var (
	ErrEmptyPackagePath = errors.New("java provider package path is empty")
	ErrPackageNotFound  = errors.New("java package not found")
	ErrEmptyModulePath  = errors.New("java provider module path is empty")
)
