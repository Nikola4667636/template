package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"template/internal/database"
	"template/internal/handler"
	"template/internal/repository"
	"template/internal/server"
	"template/internal/service"
	"template/internal/validate"
)

func main() {
	if err := run(); err != nil {
		slog.Error("service failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := validate.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	setupLogger(cfg.LOG_LEVEL)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbPool, err := database.NewPool(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}
	defer dbPool.Close()

	txManager := database.NewTxManager(dbPool, cfg.DATABASE_QUERY_TIMEOUT)
	tripRepo := repository.NewTripRepository(dbPool, cfg.DATABASE_QUERY_TIMEOUT)
	tripService := service.NewTripService(tripRepo, txManager)

	apiHandler := handler.NewAPIHandler(dbPool, tripService, cfg.DATABASE_QUERY_TIMEOUT)
	srv := server.NewServer(cfg, apiHandler)

	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("server exited with error: %w", err)
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
