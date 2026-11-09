package finish

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

func (uc *UseCase) Execute(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	var finished *trip.Trip
	if err := uc.tx.Do(ctx, func(txCtx context.Context) error {
		current, err := uc.repo.GetByIDForUpdate(txCtx, id)
		if err != nil {
			return fmt.Errorf("lock trip: %w", err)
		}

		from := current.Status
		if err := current.Finish(uc.now().UTC().Truncate(time.Microsecond)); err != nil {
			return fmt.Errorf("finish trip: %w", err)
		}
		if err := uc.repo.UpdateState(txCtx, current); err != nil {
			return fmt.Errorf("save trip state: %w", err)
		}
		if err := uc.repo.AddStatusHistory(txCtx, current.ID, &from, current.Status); err != nil {
			return fmt.Errorf("add completion status history: %w", err)
		}
		finished = current
		return nil
	}); err != nil {
		return nil, fmt.Errorf("finish trip transaction: %w", err)
	}
	return finished, nil
}
