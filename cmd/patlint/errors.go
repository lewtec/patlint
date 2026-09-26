package main

import "errors"

// Tabled CLI errors for %w wrapping and errors.Is checks.
var (
	ErrCancelled = errors.New("cancelled")
	ErrNoMatches = errors.New("no matches")
	ErrMatchSpan = errors.New("match span out of range")
)
