package main

import "errors"

// Tabled CLI errors for %w wrapping and errors.Is checks.
var (
	ErrUsage      = errors.New("usage")
	ErrCancelled  = errors.New("cancelled")
	ErrNoMatches  = errors.New("no matches")
	ErrTestFailed = errors.New("test failed")
	ErrMatchSpan  = errors.New("match span out of range")
)
