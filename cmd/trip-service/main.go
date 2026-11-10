package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Nikola4667636/template/internal/config"
	"github.com/Nikola4667636/template/internal/container"
)

func main() {
	if err := run(); err != nil {
		// Configuration may be unavailable, so fatal startup diagnostics use a bootstrap logger.
		logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
			"service", "trip-service", "trace_id", "", "span_id", "",
		)
		logger.Error("service failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, err := container.NewContainer(ctx, *cfg)
	if err != nil {
		return fmt.Errorf("build application: %w", err)
	}
	if err := app.Run(ctx); err != nil {
		return fmt.Errorf("run application: %w", err)
	}

	return nil
}
