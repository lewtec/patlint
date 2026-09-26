package kotlinref

import "errors"

var (
	ErrEmptyPackagePath = errors.New("empty kotlin package path")
	ErrPackageNotFound  = errors.New("kotlin package not found")
	ErrEmptyModulePath  = errors.New("empty kotlin module path")
)
