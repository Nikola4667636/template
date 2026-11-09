package create

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Nikola4667636/template/internal/domain/trip"
)

type UseCase struct {
	repo tripRepository
	tx   txManager
	now  func() time.Time
}

func New(repo tripRepository, tx txManager, now func() time.Time) *UseCase {
	return &UseCase{repo: repo, tx: tx, now: now}
}

func (uc *UseCase) Execute(ctx context.Context, params trip.CreateTripParams) (*trip.Trip, error) {
	if err := params.Validate(); err != nil {
		return nil, fmt.Errorf("validate create trip: %w", err)
	}

	created := &trip.Trip{
		ID:         uuid.New(),
		UserID:     params.UserID,
		DriverID:   params.DriverID,
		Status:     trip.TripStatusActive,
		StartPoint: params.StartPoint,
		EndPoint:   params.EndPoint,
		Price:      params.Price,
		StartedAt:  uc.now().UTC().Truncate(time.Microsecond),
	}

	if err := uc.tx.Do(ctx, func(txCtx context.Context) error {
		if err := uc.repo.Create(txCtx, created); err != nil {
			return fmt.Errorf("insert trip: %w", err)
		}
		if err := uc.repo.AddStatusHistory(txCtx, created.ID, nil, created.Status); err != nil {
			return fmt.Errorf("add initial status history: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("create trip transaction: %w", err)
	}
	return created, nil
}
