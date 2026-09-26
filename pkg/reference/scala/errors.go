package scalaref

import "errors"

var (
	ErrEmptyPackagePath = errors.New("empty scala package path")
	ErrPackageNotFound  = errors.New("scala package not found")
	ErrEmptyModulePath  = errors.New("empty scala module path")
)
