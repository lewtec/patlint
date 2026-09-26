package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/lewtec/lewkit/x/thread"
)

type exitCoder interface {
	ExitCode() int
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := thread.Run(ctx, Execute); err != nil {
		code := 1
		var ec exitCoder
		if errors.As(err, &ec) {
			code = ec.ExitCode()
		}
		if msg := err.Error(); msg != "" {
			if _, printErr := fmt.Fprintln(os.Stderr, err); printErr != nil {
				os.Exit(1)
			}
		}
		os.Exit(code)
	}
}
