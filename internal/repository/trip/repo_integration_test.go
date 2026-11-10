//go:build integration

package trip_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
	"github.com/Nikola4667636/template/internal/usecase/trip/create"
	"github.com/Nikola4667636/template/internal/usecase/trip/finish"
)

func TestRepositoryCreateAndGet(t *testing.T) {
	tests := []struct {
		name  string
		start domain.Coordinates
		end   domain.Coordinates
		price int64
	}{
		{
			name:  "all fields and bigint price",
			start: domain.Coordinates{Latitude: 59.9398, Longitude: 30.3146},
			end:   domain.Coordinates{Latitude: 59.929, Longitude: 30.3626},
			price: 9_000_000_000,
		},
		{name: "zero coordinates and price"},
		{
			name:  "coordinate boundaries",
			start: domain.Coordinates{Latitude: -90, Longitude: -180},
			end:   domain.Coordinates{Latitude: 90, Longitude: 180},
			price: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDatabase(t)
			want := newTrip()
			want.StartPoint, want.EndPoint, want.Price = tt.start, tt.end, tt.price
			if err := db.repo.Create(t.Context(), want); err != nil {
				t.Fatalf("insert trip without a transaction: %v", err)
			}
			got, err := db.repo.GetByID(t.Context(), want.ID)
			if err != nil {
				t.Fatalf("fetch inserted trip: %v", err)
			}
			assertTrip(t, got, want)
		})
	}
}

func TestRepositoryMissingTrip(t *testing.T) {
	db := newTestDatabase(t)
	id := uuid.New()
	got, err := db.repo.GetByID(t.Context(), id)
	if !errors.Is(err, domain.ErrTripNotFound) || got != nil {
		t.Fatalf("missing trip = %v, %v; want nil, ErrTripNotFound", got, err)
	}
	err = db.tx.Do(t.Context(), func(ctx context.Context) error {
		_, err := db.repo.GetByIDForUpdate(ctx, id)
		return err
	})
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Errorf("lock missing trip error = %v, want ErrTripNotFound", err)
	}
	missing := newTrip()
	missing.ID = id
	if err := db.repo.UpdateState(t.Context(), missing); !errors.Is(err, domain.ErrTripNotFound) {
		t.Errorf("update missing trip error = %v, want ErrTripNotFound", err)
	}
}

func TestRepositoryDriverBusy(t *testing.T) {
	db := newTestDatabase(t)
	first := newTrip()
	db.saveTrip(t, first)
	second := newTrip()
	second.DriverID = first.DriverID
	if err := db.repo.Create(t.Context(), second); !errors.Is(err, domain.ErrDriverBusy) {
		t.Fatalf("second active trip error = %v, want ErrDriverBusy", err)
	}
	db.assertNoTripOrHistory(t, second.ID)
	got, err := db.repo.GetByID(t.Context(), first.ID)
	if err != nil {
		t.Fatalf("fetch original trip after conflict: %v", err)
	}
	assertTrip(t, got, first)
	db.assertHistory(t, first.ID, statusChange{to: string(domain.TripStatusActive)})
}

func TestRepositoryDuplicateIDPreservesDatabaseError(t *testing.T) {
	db := newTestDatabase(t)
	first := newTrip()
	db.saveTrip(t, first)
	duplicate := newTrip()
	duplicate.ID = first.ID
	err := db.repo.Create(t.Context(), duplicate)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "trips_pkey" {
		t.Fatalf("duplicate ID error = %v, want primary key unique violation", err)
	}
	if errors.Is(err, domain.ErrDriverBusy) {
		t.Error("primary key conflict was incorrectly mapped to ErrDriverBusy")
	}
}

func TestRepositoryHistoryForeignKey(t *testing.T) {
	db := newTestDatabase(t)
	id := uuid.New()
	err := db.repo.AddStatusHistory(t.Context(), id, nil, domain.TripStatusActive)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("history without a trip error = %v, want foreign key violation", err)
	}
	db.assertNoTripOrHistory(t, id)
}

func TestCreateUseCasePersistsTripAndHistory(t *testing.T) {
	db := newTestDatabase(t)
	want := newTrip()
	now := want.StartedAt.Add(789 * time.Nanosecond)
	uc := create.New(db.repo, db.tx, func() time.Time { return now })
	params := domain.CreateTripParams{
		UserID: want.UserID, DriverID: want.DriverID,
		StartPoint: want.StartPoint, EndPoint: want.EndPoint, Price: want.Price,
	}
	created, err := uc.Execute(t.Context(), params)
	if err != nil {
		t.Fatalf("execute create with PostgreSQL: %v", err)
	}
	if created == nil || created.ID == uuid.Nil {
		t.Fatal("create did not return a trip with a generated ID")
	}
	want.ID = created.ID
	assertTrip(t, created, want)
	persisted, err := db.repo.GetByID(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("fetch committed new trip: %v", err)
	}
	assertTrip(t, persisted, want)
	db.assertHistory(t, created.ID, statusChange{to: string(domain.TripStatusActive)})

	conflict, err := uc.Execute(t.Context(), params)
	if !errors.Is(err, domain.ErrDriverBusy) || conflict != nil {
		t.Errorf("create for a busy driver = %v, %v; want nil, ErrDriverBusy", conflict, err)
	}
	db.assertHistory(t, created.ID, statusChange{to: string(domain.TripStatusActive)})
}

func TestFinishUseCasePersistsStateAndHistory(t *testing.T) {
	db := newTestDatabase(t)
	want := newTrip()
	db.saveTrip(t, want)
	now := want.StartedAt.Add(time.Hour + 789*time.Nanosecond)
	uc := finish.New(db.repo, db.tx, func() time.Time { return now })
	finished, err := uc.Execute(t.Context(), want.ID)
	if err != nil {
		t.Fatalf("execute finish with PostgreSQL: %v", err)
	}
	finishedAt := now.UTC().Truncate(time.Microsecond)
	want.Status, want.FinishedAt = domain.TripStatusCompleted, &finishedAt
	assertTrip(t, finished, want)
	persisted, err := db.repo.GetByID(t.Context(), want.ID)
	if err != nil {
		t.Fatalf("fetch committed completed trip: %v", err)
	}
	assertTrip(t, persisted, want)
	active := string(domain.TripStatusActive)
	history := []statusChange{{to: active}, {from: &active, to: string(domain.TripStatusCompleted)}}
	db.assertHistory(t, want.ID, history...)

	now = now.Add(time.Hour)
	repeated, err := uc.Execute(t.Context(), want.ID)
	if !errors.Is(err, domain.ErrTripCompleted) || repeated != nil {
		t.Fatalf("repeat finish = %v, %v; want nil, ErrTripCompleted", repeated, err)
	}
	persisted, err = db.repo.GetByID(t.Context(), want.ID)
	if err != nil {
		t.Fatalf("fetch trip after repeat finish: %v", err)
	}
	assertTrip(t, persisted, want)
	db.assertHistory(t, want.ID, history...)

	next := newTrip()
	next.DriverID = want.DriverID
	db.saveTrip(t, next)
	persisted, err = db.repo.GetByID(t.Context(), next.ID)
	if err != nil {
		t.Fatalf("fetch next trip of the released driver: %v", err)
	}
	assertTrip(t, persisted, next)
}

func TestRepositoryCountActive(t *testing.T) {
	db := newTestDatabase(t)
	assertCount := func(want int64) {
		t.Helper()
		got, err := db.repo.CountActive(t.Context())
		if err != nil || got != want {
			t.Fatalf("active trips = %d, %v; want %d", got, err, want)
		}
	}
	assertCount(0)
	first, second := newTrip(), newTrip()
	db.saveTrip(t, first)
	db.saveTrip(t, second)
	assertCount(2)
	uc := finish.New(db.repo, db.tx, func() time.Time { return first.StartedAt.Add(time.Hour) })
	if _, err := uc.Execute(t.Context(), first.ID); err != nil {
		t.Fatal(err)
	}
	assertCount(1)
}
