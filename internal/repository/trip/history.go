package trip

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
)

func (r *TripRepository) AddStatusHistory(ctx context.Context, tripID uuid.UUID, from *domain.TripStatus, to domain.TripStatus) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	var fromStatus any
	if from != nil {
		fromStatus = string(*from)
	}

	query, args, err := psql.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status").
		Values(tripID, fromStatus, string(to)).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build insert status history query: %w", err)
	}

	if _, err := r.executor(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to insert status history: %w", err)
	}

	return nil
}
