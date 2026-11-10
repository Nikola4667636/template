//go:build integration

package trip_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
	"github.com/Nikola4667636/template/internal/infra/postgres/transaction"
	repository "github.com/Nikola4667636/template/internal/repository/trip"
)

type testDatabase struct {
	pool         *pgxpool.Pool
	repo         *repository.TripRepository
	tx           *transaction.TxManager
	queryTimeout time.Duration
}

type statusChange struct {
	from *string
	to   string
}

func newTestDatabase(t *testing.T) *testDatabase {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required; start the lab environment and run make test-integration")
	}
	connectTimeout := testDuration(t, "DATABASE_CONNECT_TIMEOUT")
	queryTimeout := testDuration(t, "DATABASE_QUERY_TIMEOUT")
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("DATABASE_URL is not a valid PostgreSQL connection string")
	}
	cfg.ConnConfig.ConnectTimeout = connectTimeout
	cfg.MaxConns = 2
	cfg.MinConns = 0

	admin, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("open administrative pool: %v", err)
	}
	t.Cleanup(admin.Close)
	connectCtx, cancelConnect := context.WithTimeout(t.Context(), connectTimeout)
	err = admin.Ping(connectCtx)
	cancelConnect()
	if err != nil {
		t.Fatalf("PostgreSQL is unavailable; start the lab environment: %v", err)
	}

	schema := "trip_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	schemaCtx, cancelSchema := context.WithTimeout(t.Context(), queryTimeout)
	_, err = admin.Exec(schemaCtx, "CREATE SCHEMA "+quotedSchema)
	cancelSchema()
	if err != nil {
		t.Fatalf("create isolated test schema: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), queryTimeout)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("remove test schema %s: %v", schema, err)
		}
	})

	isolatedCfg := cfg.Copy()
	isolatedCfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(t.Context(), isolatedCfg)
	if err != nil {
		t.Fatalf("open isolated test pool: %v", err)
	}
	t.Cleanup(pool.Close)

	migrationDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() {
		if err := migrationDB.Close(); err != nil {
			t.Errorf("close migration connection: %v", err)
		}
	})
	provider, err := goose.NewProvider(goose.DialectPostgres, migrationDB,
		os.DirFS(filepath.Join("..", "..", "..", "migrations")), goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatalf("prepare project migrations: %v", err)
	}
	migrationCtx, cancelMigration := context.WithTimeout(t.Context(), queryTimeout)
	_, err = provider.Up(migrationCtx)
	cancelMigration()
	if err != nil {
		t.Fatalf("apply project migrations to test schema: %v", err)
	}

	return &testDatabase{
		pool:         pool,
		repo:         repository.NewTripRepository(pool, queryTimeout),
		tx:           transaction.NewTxManager(pool, queryTimeout),
		queryTimeout: queryTimeout,
	}
}

func testDuration(t *testing.T, name string) time.Duration {
	t.Helper()
	duration, err := time.ParseDuration(os.Getenv(name))
	if err != nil || duration <= 0 {
		t.Fatalf("%s must be a positive duration; run make test-integration with the lab .env", name)
	}
	return duration
}

func newTrip() *domain.Trip {
	return &domain.Trip{
		ID:         uuid.New(),
		UserID:     uuid.New(),
		DriverID:   uuid.New(),
		Status:     domain.TripStatusActive,
		StartPoint: domain.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
		EndPoint:   domain.Coordinates{Latitude: 59.929, Longitude: 30.3626},
		Price:      9_000_000_000,
		StartedAt:  time.Date(2026, 11, 10, 15, 0, 0, 123456000, time.FixedZone("UTC+3", 3*60*60)),
	}
}

func (db *testDatabase) writeTripAndHistory(ctx context.Context, trip *domain.Trip) error {
	if err := db.repo.Create(ctx, trip); err != nil {
		return fmt.Errorf("create test trip: %w", err)
	}
	if err := db.repo.AddStatusHistory(ctx, trip.ID, nil, trip.Status); err != nil {
		return fmt.Errorf("write test history: %w", err)
	}
	return nil
}

func (db *testDatabase) saveTrip(t *testing.T, trip *domain.Trip) {
	t.Helper()
	if err := db.tx.Do(t.Context(), func(ctx context.Context) error {
		return db.writeTripAndHistory(ctx, trip)
	}); err != nil {
		t.Fatalf("save test trip and history: %v", err)
	}
}

func assertTrip(t *testing.T, got, want *domain.Trip) {
	t.Helper()
	if got == nil {
		t.Fatal("received a nil trip")
	}
	if got.ID != want.ID || got.UserID != want.UserID || got.DriverID != want.DriverID ||
		got.Status != want.Status || got.StartPoint != want.StartPoint || got.EndPoint != want.EndPoint || got.Price != want.Price {
		t.Errorf("trip fields = %+v, want %+v", got, want)
	}
	if !got.StartedAt.Equal(want.StartedAt) || got.StartedAt.Location() != time.UTC {
		t.Errorf("started_at = %v, want %v in UTC", got.StartedAt, want.StartedAt)
	}
	if want.FinishedAt == nil {
		if got.FinishedAt != nil {
			t.Errorf("finished_at = %v, want nil", got.FinishedAt)
		}
		return
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(*want.FinishedAt) || got.FinishedAt.Location() != time.UTC {
		t.Errorf("finished_at = %v, want %v in UTC", got.FinishedAt, want.FinishedAt)
	}
}

func (db *testDatabase) assertHistory(t *testing.T, id uuid.UUID, want ...statusChange) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), db.queryTimeout)
	defer cancel()
	rows, err := db.pool.Query(ctx,
		"SELECT from_status, to_status FROM trip_status_history WHERE trip_id = $1 ORDER BY id", id)
	if err != nil {
		t.Fatalf("read persisted status history: %v", err)
	}
	defer rows.Close()
	var got []statusChange
	for rows.Next() {
		var change statusChange
		if err := rows.Scan(&change.from, &change.to); err != nil {
			t.Fatalf("scan persisted status change: %v", err)
		}
		got = append(got, change)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate status history: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("status history = %#v, want %#v", got, want)
	}
}

func (db *testDatabase) assertNoTripOrHistory(t *testing.T, id uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), db.queryTimeout)
	defer cancel()
	var trips, history int
	err := db.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM trips WHERE id = $1),
		(SELECT count(*) FROM trip_status_history WHERE trip_id = $1)`, id).Scan(&trips, &history)
	if err != nil {
		t.Fatalf("check rolled back rows: %v", err)
	}
	if trips != 0 || history != 0 {
		t.Errorf("rows after rollback: trips=%d, history=%d; want both zero", trips, history)
	}
}
