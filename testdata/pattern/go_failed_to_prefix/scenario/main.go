package main

import "fmt"

func open(err error) error {
	return fmt.Errorf("failed to open image: %w", err)
}

func ok(err error) error {
	return fmt.Errorf("open image: %w", err)
}
