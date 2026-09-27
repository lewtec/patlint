package main

import (
	"errors"
	stderrors "errors"
	"fmt"
)

func hitOneLine() error {
	return fmt.Errorf("boom: %w", errors.New("inner"))
}

func hitMultiLine() error {
	return fmt.Errorf(
		"boom: %w",
		errors.New("inner"),
	)
}

func hitAlias() error {
	return fmt.Errorf("x: %w", stderrors.New("y"))
}

func hitThreeArgs() error {
	return fmt.Errorf("a %v %w", 1, errors.New("z"))
}

func missWrapExisting() error {
	err := errors.New("pre")
	return fmt.Errorf("wrap: %w", err)
}

func missIndirect() error {
	f := errors.New
	return fmt.Errorf("no: %w", f("x"))
}
