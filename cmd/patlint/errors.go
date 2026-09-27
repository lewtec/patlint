package main

import "errors"

// Tabled CLI errors for %w wrapping and errors.Is checks.
var (
	ErrMatchSpan = errors.New("match span out of range")
)
