package container

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nikola4667636/template/internal/config"
	transport "github.com/Nikola4667636/template/internal/http"
	"github.com/Nikola4667636/template/internal/http/health"
	"github.com/Nikola4667636/template/internal/http/middleware"
	"github.com/Nikola4667636/template/internal/http/ready"
	createhandler "github.com/Nikola4667636/template/internal/http/trip/create"
	finishhandler "github.com/Nikola4667636/template/internal/http/trip/finish"
	gethandler "github.com/Nikola4667636/template/internal/http/trip/get"
	"github.com/Nikola4667636/template/internal/infra/postgres"
	"github.com/Nikola4667636/template/internal/infra/postgres/transaction"
	"github.com/Nikola4667636/template/internal/infra/telemetry"
	triprepository "github.com/Nikola4667636/template/internal/repository/trip"
	"github.com/Nikola4667636/template/internal/usecase/trip/create"
	"github.com/Nikola4667636/template/internal/usecase/trip/finish"
	"github.com/Nikola4667636/template/internal/usecase/trip/get"
)

type Container struct {
	pool            *pgxpool.Pool
	server          *transport.Server
	telemetry       *telemetry.Runtime
	shutdownTimeout time.Duration
	closeOnce       sync.Once
	closeErr        error
}

func NewContainer(ctx context.Context, cfg config.Config) (_ *Container, buildErr error) {
	runtime, err := telemetry.New(ctx, &cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize telemetry: %w", err)
	}
	c := &Container{telemetry: runtime, shutdownTimeout: cfg.SHUTDOWN_TIMEOUT}
	defer func() {
		if buildErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.SHUTDOWN_TIMEOUT)
			defer cancel()
			buildErr = errors.Join(buildErr, c.Close(cleanupCtx))
		}
	}()
	tracer, err := postgres.NewTracer(runtime.Traces, runtime.Meters)
	if err != nil {
		return nil, err
	}
	dbPool, err := postgres.NewPool(ctx, &cfg, tracer)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}
	c.pool = dbPool

	txManager := transaction.NewTxManager(dbPool, cfg.DATABASE_QUERY_TIMEOUT)
	tripRepo := triprepository.NewTripRepository(dbPool, cfg.DATABASE_QUERY_TIMEOUT)
	metrics, err := telemetry.NewTripMetrics(runtime.Meters.Meter("trip-service/trips"), runtime.Logger, tripRepo)
	if err != nil {
		return nil, err
	}
	createTrip := telemetry.ObserveCreate(create.New(tripRepo, txManager, time.Now), metrics)
	getTrip := get.New(tripRepo)
	finishTrip := telemetry.ObserveFinish(finish.New(tripRepo, txManager, time.Now), metrics)

	logger := runtime.Logger
	createHandler := createhandler.New(createTrip, logger)
	getHandler := gethandler.New(getTrip, logger)
	finishHandler := finishhandler.New(finishTrip, logger)
	healthHandler := health.New()
	readyHandler := ready.New(dbPool, cfg.DATABASE_QUERY_TIMEOUT)
	httpMetrics, err := middleware.NewHTTPMetrics(runtime.Meters.Meter("trip-service/http"))
	if err != nil {
		return nil, err
	}
	router := transport.NewRouter(createHandler, getHandler, finishHandler, healthHandler, readyHandler, logger, runtime.Traces, httpMetrics)
	c.server = transport.NewServer(&cfg, router, logger)
	return c, nil
}

func (c *Container) Run(ctx context.Context) error {
	c.telemetry.Logger.InfoContext(ctx, "service started")
	errCh := make(chan error, 1)
	go func() { errCh <- c.server.Run(ctx) }()
	var runErr error
	var serverExited bool
	select {
	case runErr = <-errCh:
		serverExited = true
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.shutdownTimeout)
	defer cancel()
	shutdownErr := c.server.Shutdown(shutdownCtx)
	if !serverExited {
		select {
		case runErr = <-errCh:
		case <-shutdownCtx.Done():
			runErr = shutdownCtx.Err()
		}
	}
	closeErr := c.Close(shutdownCtx)
	if shutdownCtx.Err() != nil {
		c.telemetry.Logger.ErrorContext(shutdownCtx, "shutdown budget exceeded", "timeout", c.shutdownTimeout)
	}
	if err := errors.Join(runErr, shutdownErr, closeErr); err != nil {
		return fmt.Errorf("stop application: %w", err)
	}
	c.telemetry.Logger.InfoContext(shutdownCtx, "service stopped")
	return nil
}

func (c *Container) Close(ctx context.Context) error {
	c.closeOnce.Do(func() {
		if err := c.telemetry.Shutdown(ctx); err != nil {
			c.telemetry.Logger.WarnContext(ctx, "telemetry shutdown incomplete", "error", err)
			c.closeErr = ctx.Err()
		}
		if c.pool != nil {
			closed := make(chan struct{})
			go func() { c.pool.Close(); close(closed) }()
			select {
			case <-closed:
			case <-ctx.Done():
				c.closeErr = errors.Join(c.closeErr, ctx.Err())
			}
		}
	})
	return c.closeErr
}
