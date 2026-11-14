package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := execute(ctx, os.Args[1:]); err != nil {
		writeCLIError(os.Stderr, err, os.Getenv("SRCPACK_DEBUG") == "1")
		var exitCoder interface{ ExitCode() int }
		if errors.As(err, &exitCoder) {
			return exitCoder.ExitCode()
		}
		if isCLIUsageError(err) {
			return 2
		}
		return 1
	}
	return 0
}
