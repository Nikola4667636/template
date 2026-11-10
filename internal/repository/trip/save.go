package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
)

const (
	pgUniqueViolation       = "23505"
	driverActiveUniqueIndex = "idx_trips_driver_active"
)

func (r *TripRepository) Create(ctx context.Context, trip *domain.Trip) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Insert("trips").
		Columns(
			"id", "user_id", "driver_id", "status",
			"start_latitude", "start_longitude", "end_latitude", "end_longitude",
			"price", "started_at",
		).
		Values(
			trip.ID, trip.UserID, trip.DriverID, string(trip.Status),
			trip.StartPoint.Latitude, trip.StartPoint.Longitude,
			trip.EndPoint.Latitude, trip.EndPoint.Longitude,
			trip.Price, trip.StartedAt,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build insert trip query: %w", err)
	}

	if _, err := r.executor(ctx).Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == driverActiveUniqueIndex {
			return fmt.Errorf("create trip: %w", domain.ErrDriverBusy)
		}
		return fmt.Errorf("failed to insert trip: %w", err)
	}

	return nil
}

func (r *TripRepository) UpdateState(ctx context.Context, trip *domain.Trip) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Update("trips").
		Set("status", string(trip.Status)).
		Set("finished_at", trip.FinishedAt).
		Set("updated_at", sq.Expr("NOW()")).
		Where("id = ?", trip.ID).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update trip state query: %w", err)
	}

	tag, err := r.executor(ctx).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update trip state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update trip state: %w", domain.ErrTripNotFound)
	}
	return nil
}
