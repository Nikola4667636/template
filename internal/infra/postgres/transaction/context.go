package transaction

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type txKey struct{}

func GetExecutor(ctx context.Context, fallback dbExecutor) dbExecutor {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return fallback
}
