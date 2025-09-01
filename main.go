// Runs a container reaper that listens for connections and prunes resources based on the filters received.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// run creates and runs a reaper which is cancelled when a signal is received.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	r, err := newReaper(ctx)
	if err != nil {
		return fmt.Errorf("new reaper: %w", err)
	}

	if err = r.run(ctx); err != nil {
		return fmt.Errorf("run: %w", err)
	}

	return nil
}

func main() {
	err := run()
	if err != nil {
		slog.Error("run", fieldError, err)
		os.Exit(1)
	}
}
