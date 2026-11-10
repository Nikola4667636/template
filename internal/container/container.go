package container

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nikola4667636/template/internal/config"
	transport "github.com/Nikola4667636/template/internal/http"
	"github.com/Nikola4667636/template/internal/http/health"
	"github.com/Nikola4667636/template/internal/http/ready"
	createhandler "github.com/Nikola4667636/template/internal/http/trip/create"
	finishhandler "github.com/Nikola4667636/template/internal/http/trip/finish"
	gethandler "github.com/Nikola4667636/template/internal/http/trip/get"
	"github.com/Nikola4667636/template/internal/infra/postgres"
	"github.com/Nikola4667636/template/internal/infra/postgres/transaction"
	triprepository "github.com/Nikola4667636/template/internal/repository/trip"
	"github.com/Nikola4667636/template/internal/server"
	"github.com/Nikola4667636/template/internal/usecase/trip/create"
	"github.com/Nikola4667636/template/internal/usecase/trip/finish"
	"github.com/Nikola4667636/template/internal/usecase/trip/get"
)

type Container struct {
	pool   *pgxpool.Pool
	server *server.Server
}

func NewContainer(ctx context.Context, cfg config.Config) (*Container, error) {
	dbPool, err := postgres.NewPool(ctx, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	txManager := transaction.NewTxManager(dbPool, cfg.DATABASE_QUERY_TIMEOUT)
	tripRepo := triprepository.NewTripRepository(dbPool, cfg.DATABASE_QUERY_TIMEOUT)
	createTrip := create.New(tripRepo, txManager, time.Now)
	getTrip := get.New(tripRepo)
	finishTrip := finish.New(tripRepo, txManager, time.Now)

	logger := slog.Default()
	createHandler := createhandler.New(createTrip, logger)
	getHandler := gethandler.New(getTrip, logger)
	finishHandler := finishhandler.New(finishTrip, logger)
	healthHandler := health.New()
	readyHandler := ready.New(dbPool, cfg.DATABASE_QUERY_TIMEOUT)
	router := transport.NewRouter(createHandler, getHandler, finishHandler, healthHandler, readyHandler, logger)
	srv := server.NewServer(&cfg, router)

	return &Container{
		pool:   dbPool,
		server: srv,
	}, nil
}

func (c *Container) Run(ctx context.Context) error {
	if err := c.server.Run(ctx); err != nil {
		return fmt.Errorf("server exited with error: %w", err)
	}
	return nil
}

func (c *Container) Close() {
	c.pool.Close()
}
