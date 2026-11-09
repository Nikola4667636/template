package container

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nikola4667636/template/internal/database"
	"github.com/Nikola4667636/template/internal/handler"
	"github.com/Nikola4667636/template/internal/repository"
	"github.com/Nikola4667636/template/internal/server"
	"github.com/Nikola4667636/template/internal/service"
	"github.com/Nikola4667636/template/internal/validate"
)

type Container struct {
	pool   *pgxpool.Pool
	server *server.Server
}

func NewContainer(ctx context.Context, cfg validate.Config) (*Container, error) {
	dbPool, err := database.NewPool(ctx, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	txManager := database.NewTxManager(dbPool, cfg.DATABASE_QUERY_TIMEOUT)
	tripRepo := repository.NewTripRepository(dbPool, cfg.DATABASE_QUERY_TIMEOUT)
	tripService := service.NewTripService(tripRepo, txManager)

	apiHandler := handler.NewAPIHandler(dbPool, tripService, cfg.DATABASE_QUERY_TIMEOUT)
	srv := server.NewServer(&cfg, apiHandler)

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
