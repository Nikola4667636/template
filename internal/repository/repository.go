package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"template/internal/database"
	"template/internal/domain"
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
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Select(tripColumns...).
		From("trips").
		Where("id = ?", id).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select trip query: %w", err)
	}

	trip, err := scanTrip(database.GetExecutor(ctx, r.pool).QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTripNotFound
		}
		return nil, fmt.Errorf("failed to get trip: %w", err)
	}

	return trip, nil
}

func (r *TripRepository) Finish(ctx context.Context, id uuid.UUID) (*domain.Trip, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Update("trips").
		Set("status", string(domain.TripStatusCompleted)).
		Set("finished_at", sq.Expr("NOW()")).
		Set("updated_at", sq.Expr("NOW()")).
		Where("id = ? AND status = ?", id, string(domain.TripStatusActive)).
		Suffix("RETURNING " + strings.Join(tripColumns, ", ")).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build finish trip query: %w", err)
	}

	trip, err := scanTrip(database.GetExecutor(ctx, r.pool).QueryRow(ctx, query, args...))
	if err == nil {
		return trip, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to finish trip: %w", err)
	}

	existing, getErr := r.GetByID(ctx, id)
	if getErr != nil {
		return nil, getErr
	}
	if existing.Status == domain.TripStatusCompleted {
		return nil, domain.ErrTripCompleted
	}
	return nil, fmt.Errorf("trip %s is active but was not updated", id)
}

func (r *TripRepository) AddPosition(ctx context.Context, pos *domain.TripPosition) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Insert("trip_positions").
		Columns("id", "trip_id", "latitude", "longitude", "created_at").
		Values(pos.ID, pos.TripID, pos.Point.Latitude, pos.Point.Longitude, pos.CreatedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build insert position query: %w", err)
	}

	if _, err := database.GetExecutor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to insert trip position: %w", err)
	}

	return nil
}

func (r *TripRepository) ListPositions(ctx context.Context, tripID uuid.UUID) ([]domain.TripPosition, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Select("id", "trip_id", "latitude", "longitude", "created_at").
		From("trip_positions").
		Where("trip_id = ?", tripID).
		OrderBy("created_at ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build list positions query: %w", err)
	}

	rows, err := database.GetExecutor(ctx, r.pool).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query trip positions: %w", err)
	}
	defer rows.Close()

	var positions []domain.TripPosition
	for rows.Next() {
		var p domain.TripPosition
		if err := rows.Scan(&p.ID, &p.TripID, &p.Point.Latitude, &p.Point.Longitude, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan position: %w", err)
		}
		p.CreatedAt = p.CreatedAt.UTC()
		positions = append(positions, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return positions, nil
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
