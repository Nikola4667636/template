package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
)

var tripColumns = []string{
	"id", "user_id", "driver_id", "status",
	"start_latitude", "start_longitude", "end_latitude", "end_longitude",
	"price", "started_at", "finished_at",
}

func (r *TripRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Trip, error) {
	return r.fetchByID(ctx, id, false)
}

func (r *TripRepository) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.Trip, error) {
	return r.fetchByID(ctx, id, true)
}

func (r *TripRepository) fetchByID(ctx context.Context, id uuid.UUID, forUpdate bool) (*domain.Trip, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	builder := psql.Select(tripColumns...).
		From("trips").
		Where("id = ?", id)
	if forUpdate {
		builder = builder.Suffix("FOR UPDATE")
	}
	query, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select trip query: %w", err)
	}

	trip, err := scanTrip(r.executor(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get trip by id: %w", domain.ErrTripNotFound)
		}
		return nil, fmt.Errorf("failed to get trip: %w", err)
	}

	return trip, nil
}
