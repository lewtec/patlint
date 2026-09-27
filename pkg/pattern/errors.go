package pattern

import "errors"

// Tabled package errors for %w wrapping and errors.Is checks.
var (
	ErrParse   = errors.New("pattern parse")
	ErrExpand  = errors.New("pattern expand")
	ErrCompile = errors.New("pattern compile")
	ErrMatcher = errors.New("pattern matcher")
	ErrExtract = errors.New("pattern extract")
	ErrRule    = errors.New("pattern rule")
	ErrMulti   = errors.New("pattern multi-nfa")
	ErrCoerce  = errors.New("pattern coerce")
	ErrFormat  = errors.New("pattern format")
)
