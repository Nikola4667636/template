package trip

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
)

func (r *TripRepository) CountActive(ctx context.Context) (int64, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	query, args, err := psql.Select("COUNT(*)").From("trips").
		Where(sq.Eq{"status": string(domain.TripStatusActive)}).ToSql()
	if err != nil {
		return 0, fmt.Errorf("build active trips count: %w", err)
	}
	var count int64
	if err := r.executor(ctx).QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}
	return count, nil
}
