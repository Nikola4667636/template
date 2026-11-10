//go:build integration

package trip_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	domain "github.com/Nikola4667636/template/internal/domain/trip"
)

func TestTransactionRollsBackBothTables(t *testing.T) {
	db := newTestDatabase(t)
	trip := newTrip()
	forcedErr := errors.New("abort after both inserts")
	err := db.tx.Do(t.Context(), func(ctx context.Context) error {
		if err := db.writeTripAndHistory(ctx, trip); err != nil {
			return err
		}
		return forcedErr
	})
	if !errors.Is(err, forcedErr) {
		t.Fatalf("rollback error = %v, want original callback error", err)
	}
	db.assertNoTripOrHistory(t, trip.ID)
}

func TestTransactionRollsBackWhenHistoryInsertFails(t *testing.T) {
	db := newTestDatabase(t)
	trip := newTrip()
	err := db.tx.Do(t.Context(), func(ctx context.Context) error {
		if err := db.repo.Create(ctx, trip); err != nil {
			return fmt.Errorf("prepare trip for failed history: %w", err)
		}
		return db.repo.AddStatusHistory(ctx, trip.ID, nil, domain.TripStatus("invalid"))
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("invalid history error = %v, want check constraint violation", err)
	}
	db.assertNoTripOrHistory(t, trip.ID)
}

func TestTransactionRollsBackWithCanceledContext(t *testing.T) {
	db := newTestDatabase(t)
	trip := newTrip()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	forcedErr := errors.New("abort canceled transaction")
	err := db.tx.Do(ctx, func(txCtx context.Context) error {
		if err := db.writeTripAndHistory(txCtx, trip); err != nil {
			return err
		}
		cancel()
		return forcedErr
	})
	if !errors.Is(err, forcedErr) {
		t.Fatalf("canceled rollback error = %v, want callback error", err)
	}
	db.assertNoTripOrHistory(t, trip.ID)
}

func TestNestedTransactionUsesOuterTransaction(t *testing.T) {
	db := newTestDatabase(t)
	trip := newTrip()
	forcedErr := errors.New("abort outer transaction")
	err := db.tx.Do(t.Context(), func(ctx context.Context) error {
		if err := db.repo.Create(ctx, trip); err != nil {
			return fmt.Errorf("insert in outer transaction: %w", err)
		}
		if err := db.tx.Do(ctx, func(nestedCtx context.Context) error {
			return db.repo.AddStatusHistory(nestedCtx, trip.ID, nil, trip.Status)
		}); err != nil {
			return fmt.Errorf("write history in nested transaction: %w", err)
		}
		return forcedErr
	})
	if !errors.Is(err, forcedErr) {
		t.Fatalf("nested transaction error = %v, want outer callback error", err)
	}
	db.assertNoTripOrHistory(t, trip.ID)
}

func TestTransactionRollsBackOnPanic(t *testing.T) {
	db := newTestDatabase(t)
	trip := newTrip()
	panicValue := errors.New("panic after both inserts")
	var recovered any
	var transactionErr error
	func() {
		defer func() { recovered = recover() }()
		transactionErr = db.tx.Do(t.Context(), func(ctx context.Context) error {
			if err := db.writeTripAndHistory(ctx, trip); err != nil {
				return err
			}
			panic(panicValue)
		})
	}()
	if transactionErr != nil || recovered != panicValue {
		t.Fatalf("panic rollback = %v, %v; want original panic", transactionErr, recovered)
	}
	db.assertNoTripOrHistory(t, trip.ID)
}

func TestRepositoryGetByIDForUpdateLocksRow(t *testing.T) {
	db := newTestDatabase(t)
	trip := newTrip()
	db.saveTrip(t, trip)
	err := db.tx.Do(t.Context(), func(ctx context.Context) error {
		if _, err := db.repo.GetByIDForUpdate(ctx, trip.ID); err != nil {
			return fmt.Errorf("lock trip row: %w", err)
		}
		// A second connection must fail immediately instead of waiting for the lock.
		err := tryLockTripRow(ctx, db, trip.ID)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
			return fmt.Errorf("competing row lock error = %v, want lock_not_available", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify FOR UPDATE row lock: %v", err)
	}
	if err := tryLockTripRow(t.Context(), db, trip.ID); err != nil {
		t.Errorf("row lock was not released after commit: %v", err)
	}
}

func tryLockTripRow(ctx context.Context, db *testDatabase, id uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, db.queryTimeout)
	defer cancel()
	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin competing transaction: %w", err)
	}
	defer func() {
		rollbackCtx, cancelRollback := context.WithTimeout(context.WithoutCancel(ctx), db.queryTimeout)
		defer cancelRollback()
		_ = tx.Rollback(rollbackCtx)
	}()
	var lockedID uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT id FROM trips WHERE id = $1 FOR UPDATE NOWAIT", id).Scan(&lockedID); err != nil {
		return fmt.Errorf("acquire competing row lock: %w", err)
	}
	if lockedID != id {
		return fmt.Errorf("locked trip %s, want %s", lockedID, id)
	}
	return nil
}
