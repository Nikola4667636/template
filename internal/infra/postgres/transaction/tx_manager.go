package transaction

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TxManager struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTxManager(pool *pgxpool.Pool, queryTimeout time.Duration) *TxManager {
	return &TxManager{
		pool:         pool,
		queryTimeout: queryTimeout,
	}
}

func (tm *TxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	beginCtx, cancelBegin := context.WithTimeout(ctx, tm.queryTimeout)
	tx, err := tm.pool.BeginTx(beginCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	cancelBegin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	rollbackBase := context.WithoutCancel(ctx)

	defer func() {
		if p := recover(); p != nil {
			rbCtx, cancel := context.WithTimeout(rollbackBase, tm.queryTimeout)
			_ = tx.Rollback(rbCtx)
			cancel()
			panic(p)
		}
	}()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		rbCtx, cancel := context.WithTimeout(rollbackBase, tm.queryTimeout)
		rbErr := tx.Rollback(rbCtx)
		cancel()
		if rbErr != nil {
			return fmt.Errorf("%w (rollback failed: %v)", err, rbErr)
		}
		return err
	}

	commitCtx, cancelCommit := context.WithTimeout(ctx, tm.queryTimeout)
	defer cancelCommit()
	if err := tx.Commit(commitCtx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
