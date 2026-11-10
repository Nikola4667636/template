package trip

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/Nikola4667636/template/internal/infra/postgres/transaction"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type TripRepository struct {
	db           dbExecutor
	queryTimeout time.Duration
}

func NewTripRepository(db dbExecutor, queryTimeout time.Duration) *TripRepository {
	return &TripRepository{db: db, queryTimeout: queryTimeout}
}

func (r *TripRepository) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.queryTimeout)
}

func (r *TripRepository) executor(ctx context.Context) dbExecutor {
	return transaction.GetExecutor(ctx, r.db)
}
