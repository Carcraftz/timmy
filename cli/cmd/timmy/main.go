package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"timmy/cli/internal/cli"
	"timmy/cli/internal/ssh"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := cli.New(ssh.NewRunner(), nil, os.Stdout, os.Stderr)
	if err := app.Run(ctx, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
