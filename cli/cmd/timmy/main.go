package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"timmy/cli/internal/cli"
	"timmy/cli/internal/client"
	"timmy/cli/internal/config"
	"timmy/cli/internal/ssh"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		exit(err)
	}

	httpClient, err := client.NewHTTPClient(cfg.APIURL, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		exit(err)
	}

	app := cli.New(httpClient, ssh.NewRunner(), os.Stdout, os.Stderr)
	if err := app.Run(ctx, os.Args[1:]); err != nil {
		exit(err)
	}
}

func exit(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
