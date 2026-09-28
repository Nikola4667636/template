package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"template/internal/validate"
)

func NewPool(ctx context.Context, cfg *validate.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DATABASE_URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database url: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.DATABASE_MAX_CONNS)
	poolCfg.MinConns = int32(cfg.DATABASE_MIN_CONNS)
	poolCfg.MaxConnLifetime = cfg.DATABASE_MAX_CONN_LIFETIME

	connectCtx, cancel := context.WithTimeout(ctx, cfg.DATABASE_CONNECT_TIMEOUT)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, nil
}
