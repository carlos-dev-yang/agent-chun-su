package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"chunsu/internal/cli"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cli.New(version).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "chunsu:", err)
		os.Exit(1)
	}
}
