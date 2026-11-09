package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Nikola4667636/template/internal/database"
	domain "github.com/Nikola4667636/template/internal/domain/trip"
)

const (
	pgUniqueViolation       = "23505"
	driverActiveUniqueIndex = "idx_trips_driver_active"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

var tripColumns = []string{
	"id", "user_id", "driver_id", "status",
	"start_latitude", "start_longitude", "end_latitude", "end_longitude",
	"price", "started_at", "finished_at",
}

type TripRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTripRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *TripRepository {
	return &TripRepository{pool: pool, queryTimeout: queryTimeout}
}

func (r *TripRepository) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.queryTimeout)
}

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

	if _, err := database.GetExecutor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == driverActiveUniqueIndex {
			return domain.ErrDriverBusy
		}
		return fmt.Errorf("failed to insert trip: %w", err)
	}

	return nil
}

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

	if _, err := database.GetExecutor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to insert status history: %w", err)
	}

	return nil
}

func (r *TripRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Trip, error) {
	return r.fetchByID(ctx, id, false)
}

// GetByIDForUpdate must be called within a transaction to hold the row lock.
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

	trip, err := scanTrip(database.GetExecutor(ctx, r.pool).QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get trip by id: %w", domain.ErrTripNotFound)
		}
		return nil, fmt.Errorf("failed to get trip: %w", err)
	}

	return trip, nil
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

	tag, err := database.GetExecutor(ctx, r.pool).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update trip state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update trip state: %w", domain.ErrTripNotFound)
	}
	return nil
}

func scanTrip(row pgx.Row) (*domain.Trip, error) {
	var (
		t      domain.Trip
		status string
	)

	err := row.Scan(
		&t.ID, &t.UserID, &t.DriverID, &status,
		&t.StartPoint.Latitude, &t.StartPoint.Longitude,
		&t.EndPoint.Latitude, &t.EndPoint.Longitude,
		&t.Price, &t.StartedAt, &t.FinishedAt,
	)
	if err != nil {
		return nil, err
	}

	t.Status = domain.TripStatus(status)

	t.StartedAt = t.StartedAt.UTC()
	if t.FinishedAt != nil {
		v := t.FinishedAt.UTC()
		t.FinishedAt = &v
	}

	return &t, nil
}
