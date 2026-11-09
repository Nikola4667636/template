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
		slog.Error("service failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	setupLogger(cfg.LOG_LEVEL)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, err := container.NewContainer(ctx, *cfg)
	if err != nil {
		return fmt.Errorf("build application: %w", err)
	}
	defer app.Close()

	if err := app.Run(ctx); err != nil {
		return fmt.Errorf("run application: %w", err)
	}

	slog.Info("service stopped")
	return nil
}

func setupLogger(levelName string) {
	var level slog.Level
	_ = level.UnmarshalText([]byte(levelName))

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).
		With("service", "trip-service")
	slog.SetDefault(logger)
}
